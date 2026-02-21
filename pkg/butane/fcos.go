package butane

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/coreos/butane/base/v0_6"
	"github.com/coreos/butane/config/common"
	"github.com/coreos/butane/config/fcos/v1_6"

	"github.com/netbox-community/go-netbox/v4"

	commonMachinecfg "machinecfg/pkg/common"
)

type FCOS struct {
	Config   v1_6.Config
	Hostname string
}

func CreateFCOSIgnition(client *netbox.APIClient, ctx context.Context, deviceID int32) (result string, err error) {
	var device *netbox.PaginatedDeviceWithConfigContextList
	var response *http.Response

	device, response, err = client.DcimAPI.DcimDevicesList(ctx).Id([]int32{deviceID}).Execute()

	if err != nil {
		slog.Error("CreateFCOS", "error", err.Error(), "message", response.Body)
		return result, err
	}

	if device.Count != 1 {
		slog.Error("CreateFCOS", "message", "we did not find only one result, this must not be what you expected", "device_id", deviceID, "count", device.Count)
	}

	butane, err := extractFCOSData(ctx, client, &device.Results[0])
	if err != nil {
		slog.Error("CreateFCOS", "message", err.Error())
	}
	if butane != nil {
		slog.Info(fmt.Sprintf("%v", butane))
	}

	result = GetFCOSIgnition(butane)

	return result, err
}

func CreateFCOSs(client *netbox.APIClient, ctx context.Context, filters commonMachinecfg.DeviceFilters) (result []FCOS, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList

	filters.Status = []string{"staged"}
	devices, err = commonMachinecfg.GetDevices(&ctx, client, filters)

	if devices.Count == 0 {
		slog.Warn("CreateFCOSs", "message", "no device found, this must not be what you expected")
	}

	for _, device := range devices.Results {
		butane, err := extractFCOSData(ctx, client, &device)
		if err != nil {
			slog.Error("createHardwares", "message", err.Error())
		}
		if butane != nil {
			slog.Info(fmt.Sprintf("%v", butane))
			result = append(result, FCOS{
				Config:   *butane,
				Hostname: *device.Name.Get(),
			})
		}
	}

	return result, err
}

func extractFCOSData(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) (*v1_6.Config, error) {
	var files []v0_6.File

	interfaces, _, err := c.DcimAPI.DcimInterfacesList(ctx).DeviceId([]int32{device.Id}).Execute()
	if err != nil {
		return nil, err
	}

	for _, iface := range interfaces.Results {
		ipAddresses, _, err := c.IpamAPI.IpamIpAddressesList(ctx).InterfaceId([]int32{iface.Id}).Execute()
		if err != nil {
			return nil, err
		}

		for _, ipAddr := range ipAddresses.Results {
			var vlanID int32

			prefix, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
			if err != nil {
				slog.Error("extractFCOSData", "message", err.Error())
			} else {
				if prefix.Count > 0 {
					vlanID = prefix.Results[0].Vlan.Get().Vid
					if isVlanIDinVlanList(vlanID, iface.TaggedVlans) {
						files = appendNetworkManagerFileForVlan(&ctx, c, files, vlanID, &ipAddr, &iface)
					} else {
						files = appendNetworkManagerFileForIface(&ctx, c, files, &iface, &ipAddr)
					}
				}
			}
		}
	}

	dcimFile := createDCIMFile(device)

	files = append(files, v0_6.File{Path: "/etc/dcim.yaml", Contents: v0_6.Resource{Inline: &dcimFile}})
	files = append(files, v0_6.File{Path: "/etc/hostname", Contents: v0_6.Resource{Inline: device.Name.Get()}})

	return &v1_6.Config{
		Config: v0_6.Config{
			Version: "1.6.0",
			Variant: "fcos",
			Systemd: v0_6.Systemd{},
			Storage: v0_6.Storage{Files: files},
		},
	}, nil
}

func appendNetworkManagerFileForIface(ctx *context.Context, client *netbox.APIClient, files []v0_6.File, iface *netbox.Interface, ipAddr *netbox.IPAddress) []v0_6.File {
	var content string

	content = fmt.Sprintf("[connection]\nid=%s\ntype=ethernet\ninterface-name=%s\n", iface.Name, iface.Name)

	if hasDHCPTag(ipAddr.GetTags()) {
		content = fmt.Sprintf("%s\n[ipv4]method=auto", content)
	} else {
		var gAddresses []string
		var dAddresses []string

		content = fmt.Sprintf("%s\n[ipv4]address1=%s,", content, ipAddr.Address)

		gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "gateway", ipAddr)
		for _, addr := range gatewayAddresses {
			gAddresses = append(gAddresses, addr.Address)
		}
		content = fmt.Sprintf("%s%s\n", content, strings.Join(gAddresses, ","))

		content = fmt.Sprintf("%s\ndhcp-hostname=%s", content, iface.Name)

		dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "dns", ipAddr)
		for _, addr := range dnsAddresses {
			dAddresses = append(dAddresses, addr.Address)
		}
		content = fmt.Sprintf("%s\ndns=%s;\nmay-fail=false\nmethod=manual", content, strings.Join(dAddresses, ","))
	}

	files = append(files, v0_6.File{
		Path:     fmt.Sprintf("/etc/NetworkManager/system-connections/%s.nmconnection", iface.Name),
		Contents: v0_6.Resource{Inline: &content},
	})

	return files
}

func appendNetworkManagerFileForVlan(ctx *context.Context, client *netbox.APIClient, files []v0_6.File, vlanID int32, ipAddr *netbox.IPAddress, iface *netbox.Interface) []v0_6.File {
	var content string

	content = fmt.Sprintf("[connection]\nid=%s.%v\ntype=vlan\ninterface-name=%s.%v\n[vlan]egress-priority-map=\nflags=1\nid=%v\ningress-priority-map=\nparent=%s", iface.Name, vlanID, iface.Name, vlanID, vlanID, iface.Name)

	files = append(files, v0_6.File{
		Path:     fmt.Sprintf("/etc/NetworkManager/system-connections/%s.%v.nmconnection", iface.Name, vlanID),
		Contents: v0_6.Resource{Inline: &content},
	})

	return files
}

func PrintFCOSIgnitionFile(cfg *v1_6.Config, fileDescriptor *os.File) {
	ignitionBlob := generateFCOSIgnition(cfg)
	fmt.Fprintf(fileDescriptor, "%s", ignitionBlob)
}

func generateFCOSIgnition(cfg *v1_6.Config) (result []byte) {
	ignitionCfg, report, err := cfg.ToIgn3_5(common.TranslateOptions{})
	if err != nil {
		slog.Error("generateFCOSIgnition", "message", err.Error(), "report", report.String())
	} else {
		result, _ = json.MarshalIndent(ignitionCfg, "", "  ")
	}

	return result
}

func GetFCOSIgnition(cfg *v1_6.Config) string {
	ignitionBlob := generateFCOSIgnition(cfg)
	return fmt.Sprintf("%s", ignitionBlob)
}
