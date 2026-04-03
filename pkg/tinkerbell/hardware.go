/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package tinkerbell

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"text/template"

	"github.com/netbox-community/go-netbox/v4"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	"machinecfg/pkg/butane"
	"machinecfg/pkg/common"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"

	commonMachinecfg "machinecfg/pkg/common"
)

func CreateHardwares(client *netbox.APIClient, ctx context.Context, filters common.DeviceFilters, userDataIgnitionVariant *string) (result []tinkerbellKubeObjects.Hardware, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList

	filters.Status = []string{"staged"}
	devices, err = commonMachinecfg.GetDevices(&ctx, client, filters)
	if err != nil {
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("no device found", "func", "CreateHardwares")
	}

	for _, device := range devices.Results {
		hardware, err := extractHardwareData(ctx, client, &device)
		if err != nil {
			slog.Error("failed to extract hardware data", "func", "CreateHardwares", "error", err.Error(), "device", *device.Name.Get(), "device_id", device.Id)
		}
		if hardware != nil {
			if userDataIgnitionVariant != nil {
				switch *userDataIgnitionVariant {
				case "flatcar":
					ignition, err := butane.CreateFlatcarIgnition(client, ctx, device.GetId())
					if err == nil {
						hardware.Spec.VendorData = &ignition
					}
				case "fcos":
					ignition, err := butane.CreateFCOSIgnition(client, ctx, device.GetId())
					if err == nil {
						hardware.Spec.VendorData = &ignition
					}
				default:
					slog.Warn("unsupported ignition variant", "func", "CreateHardwares", "variant", *userDataIgnitionVariant)
				}
			}

			slog.Debug("hardware object created", "func", "CreateHardwares")
			result = append(result, *hardware)
		}
	}

	return result, err
}

func PrintDefaultYAML(hardware *tinkerbellKubeObjects.Hardware, destination *os.File) {
	yamlData, err := yaml.Marshal(hardware)

	if err != nil {
		slog.Error("failed to marshal yaml", "func", "PrintDefaultYAML", "error", err.Error())
	} else {
		fmt.Fprintf(destination, "%s", yamlData)
	}
}

func PrintExternalYAML(hardware *tinkerbellKubeObjects.Hardware, templatePath string, destination *os.File) {
	var tmpl *template.Template
	var err error

	if destination == nil {
		slog.Info("no destination given, writing to stdout", "func", "PrintExternalYAML")
		destination = os.Stdout
	}

	tmpl, err = template.New(templatePath).ParseFiles(templatePath)
	if err != nil {
		slog.Error("failed to parse template", "func", "PrintExternalYAML", "error", err.Error())
		return
	}
	err = tmpl.Execute(destination, hardware)
	if err != nil {
		slog.Error("failed to execute template", "func", "PrintExternalYAML", "error", err.Error())
		return
	}
}

func extractHardwareData(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) (*tinkerbellKubeObjects.Hardware, error) {
	var primaryMacAddress string
	allowPXE := true
	allowWorkflow := true

	if !device.PrimaryIp4.IsSet() {
		return nil, fmt.Errorf("device %s does not have any primary IPv4 address", *device.Name.Get())
	}

	ipAddress := getAddrFromCIDR(device.PrimaryIp4.Get().Address)

	primaryIpAddrResult, _, err := c.IpamAPI.IpamIpAddressesList(ctx).Id([]int32{device.PrimaryIp4.Get().Id}).Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot read the IPv4 address %d: %w", device.PrimaryIp4.Get().Id, err)
	}

	if primaryIpAddrResult.Count > 0 {
		interfaceID := primaryIpAddrResult.Results[0].AssignedObjectId.Get()
		primaryMacAddress, err = getMacAddrFromIfaceID(c, &ctx, interfaceID)
		if err != nil {
			return nil, err
		}
		primaryMacAddress = strings.ToLower(primaryMacAddress)
	}

	deviceIP := netbox.IPAddress{
		Address: device.PrimaryIp4.Get().Address,
		Display: device.PrimaryIp.Get().Display,
		Url:     device.PrimaryIp4.Get().Url,
	}

	gatewaysNetbox := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(&ctx, c, "gateway", &deviceIP)
	gateways := commonMachinecfg.FromIPAddressesToStrings(gatewaysNetbox)

	nameServersNetbox := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(&ctx, c, "dns", &deviceIP)
	nameServers := commonMachinecfg.FromIPAddressesToStrings(nameServersNetbox)

	netmask, err := cidrToNetmask(device.PrimaryIp.Get().Address)
	if err != nil {
		return nil, err
	}

	objectMeta := createMetaFromDevice(device)

	systemDisk := tinkerbellKubeObjects.Disk{
		Device: "/dev/sda",
	}

	hardwareSpec := tinkerbellKubeObjects.HardwareSpec{
		Interfaces: []tinkerbellKubeObjects.Interface{
			{
				DHCP: &tinkerbellKubeObjects.DHCP{
					MAC:         primaryMacAddress,
					Hostname:    *device.Name.Get(),
					NameServers: nameServers,
					Arch:        device.Platform.Get().Name,
					UEFI:        true,
					IP: &tinkerbellKubeObjects.IP{
						Address: ipAddress,
						Netmask: netmask,
						Gateway: strings.Join(gateways, ""),
					},
				},
				DisableDHCP: false,
				Netboot: &tinkerbellKubeObjects.Netboot{
					AllowPXE:      &allowPXE,
					AllowWorkflow: &allowWorkflow,
				},
			},
		},
		Metadata: &tinkerbellKubeObjects.HardwareMetadata{
			Instance: &tinkerbellKubeObjects.MetadataInstance{
				Hostname: *device.Name.Get(),
				ID:       primaryMacAddress,
				Ips: []*tinkerbellKubeObjects.MetadataInstanceIP{
					{
						Address: ipAddress,
						Netmask: netmask,
						Gateway: strings.Join(gateways, ""),
					},
				},
			},
			Manufacturer: &tinkerbellKubeObjects.MetadataManufacturer{
				Slug: strings.ToLower(device.DeviceType.Manufacturer.Name),
			},
		},
		Disks: []tinkerbellKubeObjects.Disk{
			systemDisk,
		},
	}

	return &tinkerbellKubeObjects.Hardware{
		TypeMeta: v1.TypeMeta{
			APIVersion: "tinkerbell.org/v1alpha1",
			Kind:       "Hardware",
		},
		ObjectMeta: objectMeta,
		Spec:       hardwareSpec,
	}, nil
}

func createMetaFromDevice(device *netbox.DeviceWithConfigContext) v1.ObjectMeta {
	var namespace string

	tenant := device.Tenant.Get()
	if tenant == nil {
		namespace = "default"
	} else {
		namespace = tenant.Slug
	}

	labels := map[string]string{
		"generated-by": "machinecfg",

		"netbox-device-id": strconv.Itoa(int(device.Id)),

		"serial": *device.Serial,
		"model":  device.DeviceType.Slug,
		"role":   device.Role.GetName(),

		"site":     device.Site.GetName(),
		"location": device.Location.Get().GetName(),
		"racks":    device.Rack.Get().GetName(),

		"tenant": device.Tenant.Get().GetName(),
		"status": string(device.Status.GetLabel()),
	}

	if cluster := device.Cluster.Get(); cluster != nil {
		labels["cluster"] = cluster.GetName()
	}

	return v1.ObjectMeta{
		Name:      *device.Name.Get(),
		Namespace: namespace,
		Labels:    labels,
	}
}

func getAddrFromCIDR(cidr string) string {
	parts := strings.Split(cidr, "/")
	return parts[0]
}

func getMacAddrFromIfaceID(c *netbox.APIClient, ctx *context.Context, interfaceID *int64) (result string, err error) {
	interfaceResult, _, err := c.DcimAPI.DcimInterfacesRetrieve(*ctx, int32(*interfaceID)).Execute()
	if err == nil {
		primaryMac := interfaceResult.GetPrimaryMacAddress()
		if primaryMac.MacAddress != "" {
			result = primaryMac.MacAddress
		} else {
			if len(interfaceResult.GetMacAddresses()) > 0 {
				result = interfaceResult.GetMacAddresses()[0].MacAddress
			}
		}

		if result == "" {
			err = fmt.Errorf("iface %s, id %v does not have any MAC address", interfaceResult.GetName(), interfaceResult.Id)
		}
	}
	return result, err
}

func cidrToNetmask(cidr string) (string, error) {
	_, ipNet, err := net.ParseCIDR(cidr)

	if err != nil {
		return "", err
	}

	return net.IP(ipNet.Mask).String(), nil
}

func CreateHardwaresToPrune(client *netbox.APIClient, ctx context.Context, filters common.DeviceFilters) (result []tinkerbellKubeObjects.Hardware, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList

	filters.Status = []string{"offline", "planned"}
	devices, err = commonMachinecfg.GetDevices(&ctx, client, filters)
	if err != nil {
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("no device found", "func", "CreateHardwaresToPrune")
	}

	for _, device := range devices.Results {
		hardware, err := extractHardwareData(ctx, client, &device)
		if err != nil {
			slog.Error("failed to extract hardware data", "func", "CreateHardwaresToPrune", "error", err.Error(), "device", *device.Name.Get(), "device_id", device.Id)
		}
		if hardware != nil {
			slog.Debug("hardware object added to prune list", "func", "CreateHardwaresToPrune")
			result = append(result, *hardware)
		}
	}

	return result, err
}

// ReconcileExistingHardware reconciles an already-existing Hardware object with the
// desired state derived from NetBox:
//   - If the netbox-device-id label is missing or stale, it is patched to match desired.
//   - If the Hardware carries the provisioned annotation, the corresponding NetBox device
//     is transitioned to "active".
func ReconcileExistingHardware(k8sClient client.Client, ctx context.Context, desired *tinkerbellKubeObjects.Hardware, netboxClient *netbox.APIClient) error {
	existing := &tinkerbellKubeObjects.Hardware{}
	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: desired.Namespace, Name: desired.Name}, existing); err != nil {
		return fmt.Errorf("cannot get existing Hardware %s/%s: %w", desired.Namespace, desired.Name, err)
	}

	// Reconcile all labels from the desired state onto the existing object.
	// Labels set by other controllers (e.g. Tinkerbell owner labels) are preserved
	// because we only update keys present in desired, never remove others.
	needsPatch := false
	patch := client.MergeFrom(existing.DeepCopy())
	if existing.Labels == nil {
		existing.Labels = make(map[string]string)
	}
	for key, desiredVal := range desired.Labels {
		if existing.Labels[key] != desiredVal {
			existing.Labels[key] = desiredVal
			needsPatch = true
		}
	}
	if needsPatch {
		if err := k8sClient.Patch(ctx, existing, patch); err != nil {
			return fmt.Errorf("cannot patch labels on %s/%s: %w", existing.Namespace, existing.Name, err)
		}
		slog.Info("labels reconciled", "func", "ReconcileExistingHardware", "name", existing.Name, "namespace", existing.Namespace)
	}

	if existing.Annotations[annotationProvisioned] == "true" {
		deviceID64, err := strconv.ParseInt(desired.Labels["netbox-device-id"], 10, 32)
		if err != nil {
			return fmt.Errorf("invalid netbox-device-id %q on %s/%s: %w", desired.Labels["netbox-device-id"], existing.Namespace, existing.Name, err)
		}
		updated, err := setDeviceStatus(ctx, netboxClient, int32(deviceID64), netbox.DEVICESTATUSVALUE_ACTIVE)
		if err != nil {
			return fmt.Errorf("cannot transition NetBox device %d to active: %w", deviceID64, err)
		}
		if updated {
			slog.Info("NetBox device transitioned to active", "func", "ReconcileExistingHardware", "name", existing.Name, "device_id", deviceID64)
		}
	}

	return nil
}
