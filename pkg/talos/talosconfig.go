package talos

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/netbox-community/go-netbox/v4"

	"github.com/siderolabs/talos/pkg/machinery/config/config"
	//"github.com/siderolabs/talos/pkg/machinery/config/machine"
	//"github.com/siderolabs/talos/pkg/machinery/config/types/network"
	v1alpha1 "github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"

	"gopkg.in/yaml.v3"

	commonFilters "machinecfg/pkg/common"
)

type Talos struct {
	Config   []config.Document
	Hostname string
}

func CreateTalosConfigs(client *netbox.APIClient, ctx context.Context, filters commonFilters.DeviceFilters) (result []Talos, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList
	var response *http.Response

	switch {
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "" && len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateTalosConfigs", "message", "tenants+locations", "tenants", len(filters.Tenants), "locations", len(filters.Locations))
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Location(filters.Locations).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "":
		slog.Info("CreateTalosConfigs", "message", "tenants")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateTalosConfigs", "message", "locations")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Location(filters.Locations).Role(filters.Roles).Execute()
	default:
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Role(filters.Roles).Execute()
	}

	if err != nil {
		slog.Error("CreateTalosConfigs", "error", err.Error(), "message", response.Body)
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("CreateTalosConfigs", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		talos, err := extractTalosData(ctx, client, &device)
		if err != nil {
			slog.Error("createHardwares", "message", err.Error())
		}
		if talos != nil {
			slog.Info(fmt.Sprintf("%v", talos))
			item := Talos{
				Config:   talos,
				Hostname: device.GetName(),
			}
			result = append(result, item)

		}
	}

	return result, err
}

func extractTalosData(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) (result []config.Document, err error) {
	var talosInterfaces v1alpha1.NetworkDeviceList
	var dhcpFalse bool

	netboxInterfaces, _, err := c.DcimAPI.DcimInterfacesList(ctx).DeviceId([]int32{device.Id}).Execute()
	if err != nil {
		return nil, err
	}

	for _, netboxInterface := range netboxInterfaces.Results {
		var talosVlans v1alpha1.VlanList
		var deviceCIDR string

		ipAddresses, _, err := c.IpamAPI.IpamIpAddressesList(ctx).InterfaceId([]int32{netboxInterface.Id}).Execute()
		if err != nil {
			return nil, err
		}

		if !hasDHCPTag(netboxInterface.Tags) {
			for _, ipAddr := range ipAddresses.Results {
				var vlanID int32

				prefix, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
				if err != nil {
					slog.Error("extractFlatcarData", "message", err.Error())
				} else {
					if prefix.Count > 0 {
						vlanID = prefix.Results[0].Vlan.Get().Vid
						if isVlanIDinVlanList(vlanID, netboxInterface.TaggedVlans) {

							talosVlans = append(talosVlans, &v1alpha1.Vlan{
								VlanAddresses: []string{ipAddr.Address},
								VlanDHCP:      &dhcpFalse,
								VlanID:        uint16(vlanID),
							})

						} else {
							deviceCIDR = ipAddr.Address
						}
					}
				}
			}
		}
		talosInterfaces = append(talosInterfaces, &v1alpha1.Device{
			DeviceCIDR: deviceCIDR,
			DeviceVlans: talosVlans,
			DeviceDHCP: &dhcpFalse,
			DeviceInterface: netboxInterface.Name,
		})
	}

	machine := v1alpha1.MachineConfig{
		MachineNetwork: &v1alpha1.NetworkConfig{
			NetworkHostname:   *device.Name.Get(),
			NetworkInterfaces: talosInterfaces,
		},
		MachineNodeLabels: map[string]string{
			"generated-by": "machinecfg",

			"device.netbox.org/serial": *device.Serial,
			"device.netbox.org/model":  device.DeviceType.Slug,
			"device.netbox.org/role":   device.Role.GetName(),
			"device.netbox.org/status": string(device.Status.GetLabel()),

			"topology.kubernetes.io/region": device.Site.GetName(),
			"topology.kubernetes.io/site":   device.Site.GetName(),

			"topology.netbox.org/location": device.Location.Get().GetName(),
			"topology.netbox.org/racks":    device.Rack.Get().GetName(),

			"topology.netbox.org/tenant": device.Tenant.Get().GetName(),
		},
	}

	v1alpha1Config := v1alpha1.Config{
		ConfigVersion: "v1alpha1",
		MachineConfig: &machine,
	}

	result = append(result, &v1alpha1Config)
	return result, err
}

func isVlanIDinVlanList(vlanID int32, vlans []netbox.VLAN) (result bool) {
	for _, v := range vlans {
		if v.Vid == vlanID {
			result = true
		}
	}

	return result
}

func hasDHCPTag(tags []netbox.NestedTag) (answer bool) {
	for _, tag := range tags {
		if strings.ToLower(tag.GetName()) == "dhcp" {
			answer = true
		}
	}

	return answer
}

func PrintYAMLFile(documents []config.Document, fileDescriptor *os.File) {
	for _, d := range documents {
		yamlData, err := yaml.Marshal(d)

		if err != nil {
			slog.Error("PrintYAMLFile", "message", err.Error())
		} else {
			fmt.Fprintf(fileDescriptor, "%s", yamlData)
		}
	}
}
