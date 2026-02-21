package butane

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"

	"github.com/coreos/butane/base/v0_5"
	"github.com/coreos/butane/config/common"
	"github.com/coreos/butane/config/flatcar/v1_1"

	"github.com/netbox-community/go-netbox/v4"

	commonMachinecfg "machinecfg/pkg/common"
)

type Flatcar struct {
	Config   v1_1.Config
	Hostname string
}

func CreateFlatcarIgnition(client *netbox.APIClient, ctx context.Context, deviceID int32) (result string, err error) {
	var device *netbox.PaginatedDeviceWithConfigContextList
	var response *http.Response

	device, response, err = client.DcimAPI.DcimDevicesList(ctx).Id([]int32{deviceID}).Execute()

	if err != nil {
		slog.Error("CreateFlatcar", "error", err.Error(), "message", response.Body)
		return result, err
	}

	if device.Count != 1 {
		slog.Error("CreateFlatcar", "message", "we did not find only one result, this must not be what you expected", "device_id", deviceID, "count", device.Count)
	}

	butane, err := extractFlatcarData(ctx, client, &device.Results[0])
	if err != nil {
		slog.Error("CreateFlatcar", "message", err.Error())
	}
	if butane != nil {
		butaneJson, _ := json.Marshal(butane)
		slog.Debug("CreateFlatcarIgnition", "butaneBase64", butaneJson)
	}

	result = GetFlatcarIgnition(butane)

	return result, err
}

func CreateFlatcars(client *netbox.APIClient, ctx context.Context, filters commonMachinecfg.DeviceFilters) (result []Flatcar, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList

	filters.Status = []string{"staged"}
	devices, err = commonMachinecfg.GetDevices(&ctx, client, filters)

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

		slog.Debug("extractFlatcarData", "iface", iface.Name)

		for _, ipAddr := range ipAddresses.Results {
			var vlanID int32
			var unTaggedIfaceConfigured bool

			slog.Debug("extractFlatcarData", "iface", iface.Name, "ipAddr", ipAddr.Address)

			prefix, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
			if err != nil {
				slog.Error("extractFlatcarData", "message", err.Error())
			} else {
				if prefix.Count > 0 {
					vlanID = prefix.Results[0].Vlan.Get().Vid
					if isVlanIDinVlanList(vlanID, iface.TaggedVlans) {
						files = appendSystemdNetdevFile(files, vlanID)
						files = appendSystemdNetworkFileForVlan(&ctx, c, files, vlanID, &ipAddr)
					} else {
						if unTaggedIfaceConfigured {
							return nil, fmt.Errorf("An untagged address has already been configured on this interface. That would create a duplicated systemd-networkd file for the physical interface. Please check that the requested VLANs are properly declared.")
						}
						files = appendSystemdNetworkFileForIface(&ctx, c, files, &iface, &ipAddr)
						unTaggedIfaceConfigured = true
					}
				}
			}
		}
	}

	dcimFile := createDCIMFile(device)

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

func appendSystemdNetworkFileForIface(ctx *context.Context, client *netbox.APIClient, files []v0_5.File, iface *netbox.Interface, ipAddr *netbox.IPAddress) []v0_5.File {
	var content string

	content = fmt.Sprintf("[Match]\nName=%s\n", iface.Name)

	if iface.MacAddress.Get() != nil {
		content = fmt.Sprintf("%sMACAddress=%s\n", content, *iface.MacAddress.Get())
	}

	if hasDHCPTag(ipAddr.GetTags()) {
		content = fmt.Sprintf("%s\n[Network]\nDHCP=yes\n", content)
	} else {
		content = fmt.Sprintf("%s\n[Network]\nDHCP=no\n", content)

		gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "gateway", ipAddr)
		for _, addr := range gatewayAddresses {
			content = fmt.Sprintf("%s\nGateway=%s\n", content, addr)
		}

		dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "dns", ipAddr)
		for _, addr := range dnsAddresses {
			content = fmt.Sprintf("%s\nDNS=%s\n", content, addr)
		}
	}

	path := fmt.Sprintf("/etc/systemd/network/01-%s.network", iface.Name)
	slog.Debug("appendSystemdNetworkFileForIface", "path", path, "ipAddr", ipAddr.Address)

	files = append(files, v0_5.File{
		Path:     path,
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendSystemdNetworkFileForVlan(ctx *context.Context, client *netbox.APIClient, files []v0_5.File, vlanID int32, ipAddr *netbox.IPAddress) []v0_5.File {
	var content string

	content = fmt.Sprintf("[Match]\nName=%v\n[Network]\nDHCP=no\nAddress=%s\n", vlanID, ipAddr.Address)

	gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "gateway", ipAddr)
	for _, addr := range gatewayAddresses {
		content = fmt.Sprintf("%s\nGateway=%s", content, addr.Address)
	}

	dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "dns", ipAddr)
	for _, addr := range dnsAddresses {
		content = fmt.Sprintf("%s\nDNS=%s", content, addr.Address)
	}

	path := fmt.Sprintf("/etc/systemd/network/01-vlan-%v.network", vlanID)
	slog.Debug("appendSystemdNetworkFileForVlan", "path", path, "vlanID", vlanID, "ipAddr", ipAddr.Address)

	files = append(files, v0_5.File{
		Path:     path,
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendSystemdNetdevFile(files []v0_5.File, vlanID int32) []v0_5.File {
	content := fmt.Sprintf("[NetDev]\nName=%v\nKind=vlan\n[VLAN]\nId=%v", vlanID, vlanID)

	files = append(files, v0_5.File{
		Path:     fmt.Sprintf("/etc/systemd/network/00-vlan-%v.netdev", vlanID),
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func PrintFlatcarIgnitionFile(cfg *v1_1.Config, fileDescriptor *os.File) {
	ignitionBlob := generateFlatcarIgnition(cfg)
	fmt.Fprintf(fileDescriptor, "%s", ignitionBlob)
}

func generateFlatcarIgnition(cfg *v1_1.Config) (result []byte) {
	ignitionCfg, report, err := cfg.ToIgn3_4(common.TranslateOptions{})
	if err != nil {
		cfgJson, _ := json.Marshal(cfg)
		slog.Error("generateFlatcarIgnition", "message", err.Error(), "report", report.String(), "cfg", cfgJson)
	} else {
		result, _ = json.MarshalIndent(ignitionCfg, "", "  ")
	}

	return result
}

func GetFlatcarIgnition(cfg *v1_1.Config) string {
	ignitionBlob := generateFlatcarIgnition(cfg)
	return fmt.Sprintf("%s", ignitionBlob)
}
