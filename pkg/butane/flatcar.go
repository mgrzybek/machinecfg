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

	commonMachinecfg "machinecfg/pkg/common"
)

type Flatcar struct {
	Config   v1_1.Config
	Hostname string
}

// SystemdNetworkdDevice is a systemd-networkd .network file
type SystemdNetworkdDevice struct {
	Name       string
	MACAddress string

	Network SystemdNetworkdNetworkSection
	DHCPv4  map[string]string
}

type SystemdNetworkdNetworkSection struct {
	Addresses       []string
	Gateway         string
	DNS             []string
	Domains         []string
	VLAN            []string
	DHCP            string
	DNSDefaultRoute string
}

// SystemdNetworkdNetdev is a systemd-networkd .netdev file
type SystemdNetworkdNetdev struct {
	Name string
	Kind string
	ID   int32
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

	var netDevConfs []SystemdNetworkdNetdev
	var physicalNetworkDevice SystemdNetworkdDevice

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
			slog.Debug("extractFlatcarData", "iface", iface.Name, "ipAddr", ipAddr.Address)

			prefixes, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
			if err != nil {
				slog.Error("extractFlatcarData", "message", err.Error())
			} else {
				if prefixes.Count > 0 {
					prefix := prefixes.Results[0]
					vlan := prefix.Vlan.Get()
					if isVlanIDinVlanList(vlan.Vid, iface.TaggedVlans) {
						netDevConf := SystemdNetworkdNetdev{Name: vlan.Name, Kind: "vlan", ID: vlan.Vid}
						netDevConfs = append(netDevConfs, netDevConf)
						files = appendSystemdNetworkFileForVlan(&ctx, c, files, &netDevConf, &ipAddr, &prefix)
					} else {
						physicalNetworkDevice = setValuesToNetworkDevice(&ctx, c, files, &iface, &ipAddr, &prefix)
					}
				}
			}
		}
	}

	files = appendSystemdNetdevConfs(files, netDevConfs)
	files = appendSystemdNetworkFileForIface(files, &physicalNetworkDevice, netDevConfs)

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

func setValuesToNetworkDevice(ctx *context.Context, client *netbox.APIClient, files []v0_5.File, iface *netbox.Interface, ipAddr *netbox.IPAddress, prefix *netbox.Prefix) (result SystemdNetworkdDevice) {
	result.Name = iface.Name

	if iface.MacAddress.Get() != nil {
		result.MACAddress = *iface.MacAddress.Get()
	}

	if hasDHCPTag(ipAddr.GetTags()) {
		result.Network.DHCP = "yes"
	} else {
		result.Network.DHCP = "no"
		result.Network.Addresses = append(result.Network.Addresses, ipAddr.Address)

		gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "gateway", ipAddr)
		for _, addr := range gatewayAddresses {
			result.Network.Gateway = addr.Address
		}

		dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "dns", ipAddr)
		for _, addr := range dnsAddresses {
			result.Network.DNS = append(result.Network.DNS, strings.Split(addr.Address, "/")[0])
			result.Network.Domains = append(result.Network.Domains, fmt.Sprint(prefix.CustomFields["SearchDomain"]))
			result.Network.DNSDefaultRoute = "no"
		}
	}

	return result
}

func appendSystemdNetworkFileForIface(files []v0_5.File, networkDevice *SystemdNetworkdDevice, netDevs []SystemdNetworkdNetdev) []v0_5.File {
	var content string

	content = fmt.Sprintf("[Match]\nName=%s\n", networkDevice.Name)

	if networkDevice.MACAddress != "" {
		content = fmt.Sprintf("%sMACAddress=%s\n", content, networkDevice.MACAddress)
	}

	content = fmt.Sprintf("%s\n[Network]\nLLDP=yes\nEmitLLDP=yes\n", content)

	if networkDevice.Network.DHCP == "yes" {
		content = fmt.Sprintf("%sDHCP=yes\n", content)
	} else {
		content = fmt.Sprintf("%s\nDHCP=no\n", content)

		for _, addr := range networkDevice.Network.Addresses {
			content = fmt.Sprintf("%s\nAddress=%s\n", content, addr)
		}

		if networkDevice.Network.Gateway != "" {
			content = fmt.Sprintf("%sGateway=%s\n", content, networkDevice.Network.Gateway)
		}

		for _, addr := range networkDevice.Network.DNS {
			content = fmt.Sprintf("%sDNS=%s\n", content, addr)
		}

		if len(networkDevice.Network.Domains) == 0 {
			content = fmt.Sprintf("%s\nDNSDefaultRoute=yes\n", content)
		} else {
			content = fmt.Sprintf("%s\nDomains=%s\nDNSDefaultRoute=no\n", content, strings.Join(networkDevice.Network.Domains, " "))
		}
	}

	for _, netDev := range netDevs {
		if netDev.Kind == "vlan" {
			content = fmt.Sprintf("%sVLAN=%v\n", content, netDev.Name)
		}
	}

	path := fmt.Sprintf("/etc/systemd/network/01-%s.network", networkDevice.Name)
	slog.Debug("appendSystemdNetworkFileForIface", "path", path, "content", content)

	files = append(files, v0_5.File{
		Path:     path,
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendSystemdNetworkFileForVlan(ctx *context.Context, client *netbox.APIClient, files []v0_5.File, netDev *SystemdNetworkdNetdev, ipAddr *netbox.IPAddress, prefix *netbox.Prefix) []v0_5.File {
	var content string

	content = fmt.Sprintf("[Match]\nName=%v\n[Network]\nDHCP=no\nAddress=%s\n", netDev.Name, ipAddr.Address)

	gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "gateway", ipAddr)
	for _, addr := range gatewayAddresses {
		content = fmt.Sprintf("%s\nGateway=%s\n", content, strings.Split(addr.Address, "/")[0])
	}

	dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefixOfAddr(ctx, client, "dns", ipAddr)
	for _, addr := range dnsAddresses {
		content = fmt.Sprintf("%s\nDNS=%s\n", content, strings.Split(addr.Address, "/")[0])
	}
	if len(dnsAddresses) > 0 {
		if prefix.CustomFields["Domains"] != nil {
			content = fmt.Sprintf("%s\nDomains=%sDNSDefaultRoute=no\n", content, prefix.CustomFields["Domains"])
		} else {
			content = fmt.Sprintf("%sDNSDefaultRoute=yes\n", content)
		}
	}

	path := fmt.Sprintf("/etc/systemd/network/01-%v.network", netDev.Name)
	slog.Debug("appendSystemdNetworkFileForVlan", "path", path, "content", content)

	files = append(files, v0_5.File{
		Path:     path,
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendSystemdNetdevConfs(files []v0_5.File, vlans []SystemdNetworkdNetdev) []v0_5.File {
	for _, vlan := range vlans {
		content := fmt.Sprintf("[NetDev]\nName=%v\nKind=%s\n[VLAN]\nId=%v\n", vlan.Name, vlan.Kind, vlan.ID)

		path := fmt.Sprintf("/etc/systemd/network/00-%v.netdev", vlan.Name)
		files = append(files, v0_5.File{
			Path:     path,
			Contents: v0_5.Resource{Inline: &content},
		})
		slog.Debug("appendSystemdNetdevConfs", "path", path, "content", content)
	}

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
