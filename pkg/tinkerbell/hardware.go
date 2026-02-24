package tinkerbell

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"strings"
	"text/template"

	"github.com/netbox-community/go-netbox/v4"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

	if devices.Count == 0 {
		slog.Warn("CreateHardwares", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		hardware, err := extractHardwareData(ctx, client, &device)
		if err != nil {
			slog.Error("CreateHardwares", "message", err.Error(), "device", *device.Name.Get(), "device_id", device.Id)
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
					slog.Warn("CreateHardwares", "message", "the given variant is not supported. Skipping vendorData update.", "variant", *userDataIgnitionVariant)
				}
			}

			hardwareJson, _ := json.Marshal(hardware)
			slog.Info("CreateHardwares", "hardware", hardwareJson)
			result = append(result, *hardware)
		}
	}

	return result, err
}

func PrintDefaultYAML(hardware *tinkerbellKubeObjects.Hardware, destination *os.File) {
	yamlData, err := yaml.Marshal(hardware)

	if err != nil {
		slog.Error("PrintDefaultYAML", "message", err.Error())
	} else {
		fmt.Fprintf(destination, "%s", yamlData)
	}
}

func PrintExternalYAML(hardware *tinkerbellKubeObjects.Hardware, templatePath string, destination *os.File) {
	var tmpl *template.Template
	var err error

	if destination == nil {
		slog.Info("PrintExternalYAML", "message", "writing to stdout as no destination has been given")
		destination = os.Stdout
	}

	tmpl, err = template.New(templatePath).ParseFiles(templatePath)
	if err != nil {
		slog.Error("PrintExternalYAML", "message", err.Error())
		return
	}
	err = tmpl.Execute(destination, hardware)
	if err != nil {
		slog.Error("PrintExternalYAML", "message", err.Error())
		return
	}
}

func extractHardwareData(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) (*tinkerbellKubeObjects.Hardware, error) {
	var primaryMacAddress string
	allowPXE := true
	allowWorkflow := true

	if !device.PrimaryIp4.IsSet() {
		return nil, fmt.Errorf("device %s does not have any primary IPv4 address", device.Name)
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
		namespace = strings.ToLower(tenant.Name)
	}

	return v1.ObjectMeta{
		Name:      *device.Name.Get(),
		Namespace: namespace,
		Labels: map[string]string{
			"generated-by": "machinecfg",

			"serial": *device.Serial,
			"model":  device.DeviceType.Slug,
			"role":   device.Role.GetName(),

			"site":     device.Site.GetName(),
			"location": device.Location.Get().GetName(),
			"racks":    device.Rack.Get().GetName(),

			"tenant": device.Tenant.Get().GetName(),
			"status": string(device.Status.GetLabel()),
		},
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

	if devices.Count == 0 {
		slog.Warn("CreateHardwaresToPrune", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		hardware, err := extractHardwareData(ctx, client, &device)
		if err != nil {
			slog.Error("CreateHardwaresToPrune", "message", err.Error(), "device", *device.Name.Get(), "device_id", device.Id)
		}
		if hardware != nil {
			hardwareJson, _ := json.Marshal(hardware)
			slog.Info("CreateHardwaresToPrune", "hardware", hardwareJson)
			result = append(result, *hardware)
		}
	}

	return result, err
}
