package butane

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/coreos/butane/base/v0_5"
	"github.com/coreos/butane/config/common"
	"github.com/coreos/butane/config/flatcar/v1_1"

	"github.com/netbox-community/go-netbox/v4"

	commonFilters "machinecfg/pkg/common"
)

type Flatcar struct {
	Config   v1_1.Config
	Hostname string
}

func CreateFlatcars(client *netbox.APIClient, ctx context.Context, filters commonFilters.DeviceFilters) (result []Flatcar, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList
	var response *http.Response

	switch {
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "" && len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateFlatcars", "message", "tenants+locations", "tenants", len(filters.Tenants), "locations", len(filters.Locations))
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Location(filters.Locations).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "":
		slog.Info("CreateFlatcars", "message", "tenants")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("CreateFlatcars", "message", "locations")
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Location(filters.Locations).Role(filters.Roles).Execute()
	default:
		devices, response, err = client.DcimAPI.DcimDevicesList(ctx).HasPrimaryIp(true).Status([]string{"active"}).Site(filters.Sites).Role(filters.Roles).Execute()
	}

	if err != nil {
		slog.Error("CreateFlatcars", "error", err.Error(), "message", response.Body)
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("CreateFlatcars", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		butane, err := extractFlatcarData(ctx, client, &device)
		if err != nil {
			slog.Error("createHardwares", "message", err.Error())
		}
		if butane != nil {
			slog.Info(fmt.Sprintf("%v", butane))
			result = append(result, Flatcar{
				Config:   *butane,
				Hostname: *device.Name.Get(),
			})
		}
	}

	return result, err
}

func extractFlatcarData(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) (*v1_1.Config, error) {
	var files []v0_5.File

	interfaces, _, err := c.DcimAPI.DcimInterfacesList(ctx).DeviceId([]int32{device.Id}).Execute()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces.Results {
		ipAddresses, _, err := c.IpamAPI.IpamIpAddressesList(ctx).InterfaceId([]int32{iface.Id}).Execute()
		if err != nil {
			return nil, err
		}

		if !hasDHCPTag(iface.Tags) {
			for _, ipAddr := range ipAddresses.Results {
				var vlanID int32

				prefix, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
				if err != nil {
					slog.Error("extractFlatcarData", "message", err.Error())
				} else {
					if prefix.Count > 0 {
						vlanID = prefix.Results[0].Vlan.Get().Vid
						if isVlanIDinVlanList(vlanID, iface.TaggedVlans) {
							files = appendNetdevFile(files, vlanID)
							files = appendNetworkFileForVlan(files, vlanID, &ipAddr)
						} else {
							files = appendNetworkFileForIface(files, &iface, &ipAddr)
						}
					}
				}
			}
		}
	}

	dcimFile := fmt.Sprintf(
		"---\ngenerated-by: machinecfg\nserial: %s\nmodel: %s\nrole: %s\nsite: %s\nlocation: %s\nracks: %s\ntenant: %s\nstatus: %s",
		*device.Serial,
		device.DeviceType.Slug,
		device.Role.GetName(),
		device.Site.GetName(),
		device.Location.Get().GetName(),
		device.Rack.Get().GetName(),
		device.Tenant.Get().GetName(),
		device.Status.GetLabel(),
	)

	files = append(files, v0_5.File{Path: "/etc/dcim.yaml", Contents: v0_5.Resource{Inline: &dcimFile}})
	files = append(files, v0_5.File{Path: "/etc/hostname", Contents: v0_5.Resource{Inline: device.Name.Get()}})

	return &v1_1.Config{
		Config: v0_5.Config{
			Version: "1.1.0",
			Variant: "flatcar",
			Systemd: v0_5.Systemd{},
			Storage: v0_5.Storage{Files: files},
		},
	}, nil
}

func hasDHCPTag(tags []netbox.NestedTag) (answer bool) {
	for _, tag := range tags {
		if strings.ToLower(tag.GetName()) == "dhcp" {
			answer = true
		}
	}

	return answer
}

func appendNetworkFileForIface(files []v0_5.File, iface *netbox.Interface, ipAddr *netbox.IPAddress) []v0_5.File {
	var content string

	if iface.MacAddress.Get() != nil {
		content = fmt.Sprintf("[Match]\nName=%s\nMACAddress=%s\n[Network]\nDHCP=no\nAddress=%s\nGateway=192.168.1.1\nDNS=8.8.8.8", iface.Name, *iface.MacAddress.Get(), ipAddr.Address)
	} else {
		content = fmt.Sprintf("[Match]\nName=%s\n[Network]\nDHCP=no\nAddress=%s\nGateway=192.168.1.1\nDNS=8.8.8.8", iface.Name, ipAddr.Address)
	}

	files = append(files, v0_5.File{
		Path:     fmt.Sprintf("/etc/systemd/network/01-%s.network", iface.Name),
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendNetworkFileForVlan(files []v0_5.File, vlanID int32, ipAddr *netbox.IPAddress) []v0_5.File {
	content := fmt.Sprintf("[Match]\nName=%v\n[Network]\nDHCP=no\nAddress=%s\nGateway=192.168.1.1\nDNS=8.8.8.8", vlanID, ipAddr.Address)

	files = append(files, v0_5.File{
		Path:     fmt.Sprintf("/etc/systemd/network/01-vlan-%v.network", vlanID),
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendNetdevFile(files []v0_5.File, vlanID int32) []v0_5.File {
	content := fmt.Sprintf("[NetDev]\nName=%v\nKind=vlan\n[VLAN]\nId=%v", vlanID, vlanID)

	files = append(files, v0_5.File{
		Path:     fmt.Sprintf("/etc/systemd/network/00-vlan-%v.netdev", vlanID),
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func isVlanIDinVlanList(vlanID int32, vlans []netbox.VLAN) (result bool) {
	for _, v := range vlans {
		if v.Vid == vlanID {
			result = true
		}
	}

	return result
}

func PrintIgnitionFile(cfg *v1_1.Config, fileDescriptor *os.File) {
	ignitionBlob := generateIgnition(cfg)
	fmt.Fprintf(fileDescriptor, "%s", ignitionBlob)
}

func generateIgnition(cfg *v1_1.Config) (result []byte) {
	ignitionCfg, report, err := cfg.ToIgn3_4(common.TranslateOptions{})
	if err != nil {
		slog.Error("GetIgnitionFile", "message", err.Error(), "report", report.String())
	} else {
		result, _ = json.MarshalIndent(ignitionCfg, "", "  ")
	}

	return result
}

func GetIgnition(cfg *v1_1.Config) string {
	ignitionBlob := generateIgnition(cfg)
	return fmt.Sprintf("%s", ignitionBlob)
}
