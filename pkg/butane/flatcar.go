/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package butane

import (
	"context"
	"encoding/json"
	"errors"
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
		if response != nil {
			slog.Error("failed to list devices", "func", "CreateFlatcarIgnition", "error", err.Error(), "code", response.StatusCode)
		} else {
			slog.Error("failed to list devices", "func", "CreateFlatcarIgnition", "error", err.Error())
		}
		return result, err
	}

	if device.Count != 1 {
		slog.Error("unexpected device count", "func", "CreateFlatcarIgnition", "device_id", deviceID, "count", device.Count)
	}

	butane, err := extractFlatcarData(ctx, client, &device.Results[0])
	if err != nil {
		slog.Error("failed to extract flatcar data", "func", "CreateFlatcarIgnition", "error", err.Error())
	}
	if butane != nil {
		slog.Debug("flatcar config extracted", "func", "CreateFlatcarIgnition")
	}

	result = GetFlatcarIgnition(butane)

	return result, err
}

func CreateFlatcars(client *netbox.APIClient, ctx context.Context, filters commonMachinecfg.DeviceFilters) (result []Flatcar, err error) {
	var devices *netbox.PaginatedDeviceWithConfigContextList

	filters.Status = []string{"staged"}
	devices, err = commonMachinecfg.GetDevices(&ctx, client, filters)
	if err != nil {
		return result, err
	}

	if devices.Count == 0 {
		slog.Warn("no device found", "func", "CreateFlatcars")
	}

	var extractErrs []error
	for _, device := range devices.Results {
		butane, extractErr := extractFlatcarData(ctx, client, &device)
		if extractErr != nil {
			slog.Error("failed to extract flatcar data", "func", "CreateFlatcars", "error", extractErr.Error())
			extractErrs = append(extractErrs, extractErr)
			continue
		}
		if butane != nil {
			slog.Debug("flatcar config extracted", "func", "CreateFlatcars")
			result = append(result, Flatcar{
				Config:   *butane,
				Hostname: *device.Name.Get(),
			})
		}
	}

	return result, errors.Join(extractErrs...)
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

		slog.Debug("processing interface", "func", "extractFlatcarData", "iface", iface.Name)

		for _, ipAddr := range ipAddresses.Results {
			slog.Debug("processing ip address", "func", "extractFlatcarData", "iface", iface.Name, "ipAddr", ipAddr.Address)

			prefixes, _, err := c.IpamAPI.IpamPrefixesList(ctx).Contains(ipAddr.Address).Execute()
			if err != nil {
				slog.Error("failed to list prefixes", "func", "extractFlatcarData", "error", err.Error())
			} else {
				if prefixes.Count > 0 {
					prefix := prefixes.Results[0]
					vlan := prefix.Vlan.Get()
					if vlan != nil && commonMachinecfg.IsVlanIDInVlanList(vlan.Vid, iface.TaggedVlans) {
						netDevConf := SystemdNetworkdNetdev{Name: vlan.Name, Kind: "vlan", ID: vlan.Vid}
						netDevConfs = append(netDevConfs, netDevConf)
						files = appendSystemdNetworkFileForVlan(&ctx, c, files, &netDevConf, &ipAddr, &prefix)
					} else {
						physicalNetworkDevice = setValuesToNetworkDevice(&ctx, c, &iface, &ipAddr, &prefix)
					}
				}
			}
		}
	}

	files = appendSystemdNetdevConfs(files, netDevConfs)
	files = appendSystemdNetworkFileForIface(files, &physicalNetworkDevice, netDevConfs)

	dcimFile := createDCIMFile(device)

	files = append(files, v0_5.File{Path: "/etc/dcim.yaml", Contents: v0_5.Resource{Inline: &dcimFile}})
	hostname := getHostname(ctx, c, device)
	files = append(files, v0_5.File{Path: "/etc/hostname", Contents: v0_5.Resource{Inline: &hostname}})

	return &v1_1.Config{
		Config: v0_5.Config{
			Version: "1.1.0",
			Variant: "flatcar",
			Systemd: v0_5.Systemd{},
			Storage: v0_5.Storage{Files: files},
		},
	}, nil
}

func setValuesToNetworkDevice(ctx *context.Context, client *netbox.APIClient, iface *netbox.Interface, ipAddr *netbox.IPAddress, prefix *netbox.Prefix) (result SystemdNetworkdDevice) {
	result.Name = iface.Name

	if iface.MacAddress.Get() != nil {
		result.MACAddress = *iface.MacAddress.Get()
	}

	if commonMachinecfg.HasDHCPTag(ipAddr.GetTags()) {
		result.Network.DHCP = "yes"
	} else {
		result.Network.DHCP = "no"
		result.Network.Addresses = append(result.Network.Addresses, ipAddr.Address)

		gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefix(ctx, client, "gateway", prefix)
		for _, addr := range gatewayAddresses {
			result.Network.Gateway = addr.Address
		}

		dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefix(ctx, client, "dns", prefix)
		for _, addr := range dnsAddresses {
			result.Network.DNS = append(result.Network.DNS, strings.Split(addr.Address, "/")[0])
			result.Network.DNSDefaultRoute = "no"
		}
		if len(dnsAddresses) > 0 && prefix.CustomFields["Domains"] != nil {
			result.Network.Domains = append(result.Network.Domains, fmt.Sprint(prefix.CustomFields["Domains"]))
		}
	}

	return result
}

func appendSystemdNetworkFileForIface(files []v0_5.File, networkDevice *SystemdNetworkdDevice, netDevs []SystemdNetworkdNetdev) []v0_5.File {
	var b strings.Builder

	fmt.Fprintf(&b, "[Match]\nName=%s\n", networkDevice.Name)
	if networkDevice.MACAddress != "" {
		fmt.Fprintf(&b, "MACAddress=%s\n", networkDevice.MACAddress)
	}
	fmt.Fprintf(&b, "\n[Network]\nLLDP=yes\nEmitLLDP=yes\n")

	if networkDevice.Network.DHCP == "yes" {
		fmt.Fprintf(&b, "DHCP=yes\n")
	} else {
		fmt.Fprintf(&b, "\nDHCP=no\n")
		for _, addr := range networkDevice.Network.Addresses {
			fmt.Fprintf(&b, "\nAddress=%s\n", addr)
		}
		if networkDevice.Network.Gateway != "" {
			fmt.Fprintf(&b, "Gateway=%s\n", networkDevice.Network.Gateway)
		}
		for _, addr := range networkDevice.Network.DNS {
			fmt.Fprintf(&b, "DNS=%s\n", addr)
		}
		if len(networkDevice.Network.Domains) == 0 {
			fmt.Fprintf(&b, "\nDNSDefaultRoute=yes\n")
		} else {
			fmt.Fprintf(&b, "\nDomains=%s\nDNSDefaultRoute=no\n", strings.Join(networkDevice.Network.Domains, " "))
		}
	}

	for _, netDev := range netDevs {
		if netDev.Kind == "vlan" {
			fmt.Fprintf(&b, "VLAN=%v\n", netDev.Name)
		}
	}

	path := fmt.Sprintf("/etc/systemd/network/01-%s.network", networkDevice.Name)
	slog.Debug("writing network file", "func", "appendSystemdNetworkFileForIface", "path", path)

	content := b.String()
	files = append(files, v0_5.File{
		Path:     path,
		Contents: v0_5.Resource{Inline: &content},
	})

	return files
}

func appendSystemdNetworkFileForVlan(ctx *context.Context, client *netbox.APIClient, files []v0_5.File, netDev *SystemdNetworkdNetdev, ipAddr *netbox.IPAddress, prefix *netbox.Prefix) []v0_5.File {
	var b strings.Builder

	fmt.Fprintf(&b, "[Match]\nName=%v\n[Network]\nDHCP=no\nAddress=%s\n", netDev.Name, ipAddr.Address)

	gatewayAddresses := commonMachinecfg.GetTaggedAddressesFromPrefix(ctx, client, "gateway", prefix)
	for _, addr := range gatewayAddresses {
		fmt.Fprintf(&b, "\nGateway=%s\n", strings.Split(addr.Address, "/")[0])
	}

	dnsAddresses := commonMachinecfg.GetTaggedAddressesFromPrefix(ctx, client, "dns", prefix)
	for _, addr := range dnsAddresses {
		fmt.Fprintf(&b, "\nDNS=%s\n", strings.Split(addr.Address, "/")[0])
	}
	if len(dnsAddresses) > 0 {
		if prefix.CustomFields["Domains"] != nil {
			fmt.Fprintf(&b, "\nDomains=%s\nDNSDefaultRoute=no\n", prefix.CustomFields["Domains"])
		} else {
			fmt.Fprintf(&b, "DNSDefaultRoute=yes\n")
		}
	}

	path := fmt.Sprintf("/etc/systemd/network/01-%v.network", netDev.Name)
	slog.Debug("writing vlan network file", "func", "appendSystemdNetworkFileForVlan", "path", path)

	content := b.String()
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
		slog.Debug("writing netdev file", "func", "appendSystemdNetdevConfs", "path", path)
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
		slog.Error("failed to generate ignition", "func", "generateFlatcarIgnition", "error", err.Error(), "report", report.String())
	} else {
		result, _ = json.MarshalIndent(ignitionCfg, "", "  ")
	}

	return result
}

func GetFlatcarIgnition(cfg *v1_1.Config) string {
	return string(generateFlatcarIgnition(cfg))
}
