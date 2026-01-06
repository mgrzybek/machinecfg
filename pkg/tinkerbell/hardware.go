package tinkerbell

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"text/template"

	"github.com/netbox-community/go-netbox/v4"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"machinecfg/pkg/common"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
)

func CreateHardwares(client *netbox.APIClient, ctx context.Context, filters common.DeviceFilters) (result []tinkerbellKubeObjects.Hardware, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList
	var response *http.Response

	switch {
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "" && len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateHardwares", "message", "tenants+locations", "tenants", len(filters.Tenants), "locations", len(filters.Locations))
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"staged"}).Site(filters.Sites).Location(filters.Locations).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "":
		slog.Info("CreateHardwares", "message", "tenants")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"staged"}).Site(filters.Sites).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateHardwares", "message", "locations")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"staged"}).Site(filters.Sites).Location(filters.Locations).Role(filters.Roles).Execute()
	default:
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"staged"}).Site(filters.Sites).Role(filters.Roles).Execute()
	}

	if err != nil {
		slog.Error("CreateHadwares", "error", err.Error(), "message", response.Body)
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("CreateHardwares", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		hardware, err := extractHardwareData(ctx, client, &device)
		if err != nil {
			slog.Error("CreateHardwares", "message", err.Error(), "device", *device.Name.Get(), "device_id", device.Id)
		}
		if hardware != nil {
			slog.Info(fmt.Sprintf("%v", hardware))
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

	// TODO: Get gateway and DNS from somewhere
	gateway := "192.168.1.1"
	nameServers := []string{"1.1.1.1", "8.8.8.8"}

	netmask, err := cidrToNetmask(device.PrimaryIp.Get().Address)
	if err != nil {
		return nil, err
	}

	return &tinkerbellKubeObjects.Hardware{
		TypeMeta: v1.TypeMeta{
			APIVersion: "tinkerbell.org/v1alpha1",
			Kind:       "Hardware",
		},
		ObjectMeta: v1.ObjectMeta{
			Name:      *device.Name.Get(),
			Namespace: "default",
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
		},
		Spec: tinkerbellKubeObjects.HardwareSpec{
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
							Gateway: gateway,
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
							Gateway: gateway,
						},
					},
				},
				Manufacturer: &tinkerbellKubeObjects.MetadataManufacturer{
					Slug: strings.ToLower(device.DeviceType.Manufacturer.Name),
				},
			},
		},
	}, nil
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
