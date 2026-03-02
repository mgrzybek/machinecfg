package common

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/netbox-community/go-netbox/v4"
)

func GetDevices(ctx *context.Context, client *netbox.APIClient, filters DeviceFilters) (devices *netbox.PaginatedDeviceWithConfigContextList, err error) {
	var response *http.Response

	req := client.DcimAPI.DcimDevicesList(*ctx).
		HasPrimaryIp(true).
		Status(filters.Status).
		Site(filters.Sites).
		Role(filters.Roles)

	if len(filters.Regions) > 0 && filters.Regions[0] != "" {
		req = req.Region(filters.Regions)
	}
	if len(filters.Locations) > 0 && filters.Locations[0] != "" {
		req = req.Location(filters.Locations)
	}
	if len(filters.Tenants) > 0 && filters.Tenants[0] != "" {
		req = req.Tenant(filters.Tenants)
	}
	if len(filters.Racks) > 0 && filters.Racks[0] != 0 {
		req = req.RackId(filters.Racks)
	}

	devices, response, err = req.Execute()

	if err != nil {
		if response != nil {
			slog.Error("failed to list devices", "func", "GetDevices", "error", err.Error(), "code", response.StatusCode)
		} else {
			slog.Error("failed to list devices", "func", "GetDevices", "error", err.Error())
		}
	}

	return devices, err
}

func GetTaggedAddressesFromPrefixOfAddr(ctx *context.Context, client *netbox.APIClient, tag string, addr *netbox.IPAddress) (result []netbox.IPAddress) {
	prefixes, response, err := client.IpamAPI.IpamPrefixesList(*ctx).Contains(addr.Address).Execute()

	if err != nil {
		slog.Error("failed to list prefixes", "func", "GetTaggedAddressesFromPrefixOfAddr", "error", err.Error())
	} else if response.StatusCode != 200 {
		slog.Error("unexpected response status", "func", "GetTaggedAddressesFromPrefixOfAddr", "code", response.StatusCode)
	}

	if prefixes.Count == 0 {
		slog.Warn("no prefix found", "func", "GetTaggedAddressesFromPrefixOfAddr", "ipAddress", addr.Address)
	} else {
		prefix := prefixes.Results[0]

		addresses, _, err := client.IpamAPI.IpamIpAddressesList(*ctx).Parent([]string{prefix.Display}).Tag([]string{tag}).Execute()

		if err != nil {
			slog.Warn("failed to list addresses by tag", "func", "GetTaggedAddressesFromPrefixOfAddr", "error", err.Error())
		} else {
			if addresses.Count == 0 {
				slog.Warn("no address found with tag", "func", "GetTaggedAddressesFromPrefixOfAddr", "prefix_id", prefix.Id, "prefix", prefix.Display, "tag", tag)
			}

			for _, address := range addresses.Results {
				result = append(result, address)
			}
		}
	}

	return result
}

func HasDHCPTag(tags []netbox.NestedTag) (answer bool) {
	for _, tag := range tags {
		if strings.ToLower(tag.GetName()) == "dhcp" {
			answer = true
		}
	}

	return answer
}

func IsVlanIDInVlanList(vlanID int32, vlans []netbox.VLAN) (result bool) {
	for _, v := range vlans {
		if v.Vid == vlanID {
			result = true
		}
	}

	return result
}

func FromIPAddressesToStrings(addresses []netbox.IPAddress) (result []string) {
	for _, addr := range addresses {
		addr.Address = strings.Split(addr.Address, "/")[0]
		result = append(result, addr.Address)
	}

	return result
}
