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

	switch {
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "" && len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("GetDevices", "message", "tenants+locations", "tenants", len(filters.Tenants), "locations", len(filters.Locations))
		devices, response, err = client.DcimAPI.DcimDevicesList(*ctx).HasPrimaryIp(true).Status(filters.Status).Site(filters.Sites).Location(filters.Locations).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Tenants) > 0 && filters.Tenants[0] != "":
		slog.Info("GetDevices", "message", "tenants")
		devices, response, err = client.DcimAPI.DcimDevicesList(*ctx).HasPrimaryIp(true).Status(filters.Status).Site(filters.Sites).Tenant(filters.Tenants).Role(filters.Roles).Execute()
	case len(filters.Locations) > 0 && filters.Locations[0] != "":
		slog.Info("GetDevices", "message", "locations")
		devices, response, err = client.DcimAPI.DcimDevicesList(*ctx).HasPrimaryIp(true).Status(filters.Status).Site(filters.Sites).Location(filters.Locations).Role(filters.Roles).Execute()
	default:
		devices, response, err = client.DcimAPI.DcimDevicesList(*ctx).HasPrimaryIp(true).Status(filters.Status).Site(filters.Sites).Role(filters.Roles).Execute()
	}

	if err != nil {
		slog.Error("GetDevices", "error", err.Error(), "message", response.Body, "code", response.StatusCode)
	}

	return devices, err
}

func GetTaggedAddressesFromPrefixOfAddr(ctx *context.Context, client *netbox.APIClient, tag string, addr *netbox.IPAddress) (result []netbox.IPAddress) {
	prefixes, response, err := client.IpamAPI.IpamPrefixesList(*ctx).Contains(addr.Address).Execute()

	if err != nil {
		slog.Error("getTaggedAddressesFromPrefixOfAddr", "message", err.Error())
	}

	if response.StatusCode != 200 {
		slog.Error("getTaggedAddressesFromPrefixOfAddr", "message", response.Body, "code", response.StatusCode)
	}

	if prefixes.Count == 0 {
		slog.Warn("getTaggedAddressesFromPrefixOfAddr", "message", "No prefix found. This should not happen", "ipAddress", addr.Address)
	} else {
		prefix := prefixes.Results[0]

		addresses, _, err := client.IpamAPI.IpamIpAddressesList(*ctx).Parent([]string{prefix.Display}).Tag([]string{tag}).Execute()

		if err != nil {
			slog.Warn("getTaggedAddressesFromPrefixOfAddr", "message", err.Error())
		} else {
			if addresses.Count == 0 {
				slog.Warn("getTaggedAddressesFromPrefixOfAddr", "message", "No address found with the requested tag. This should not happen", "prefix_id", prefix.Id, "prefix", prefix.Display, "tag", tag)
			}

			for _, address := range addresses.Results {
				result = append(result, address)
			}
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
