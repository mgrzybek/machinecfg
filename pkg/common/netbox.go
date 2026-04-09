/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package common

import (
	"context"
	"fmt"
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
	if len(filters.Clusters) > 0 && filters.Clusters[0] != "" {
		clusterIDs, clusterErr := resolveClusterIDs(ctx, client, filters.Clusters)
		if clusterErr != nil {
			slog.Error("failed to resolve cluster IDs", "func", "GetDevices", "error", clusterErr.Error())
			return nil, clusterErr
		}
		if len(clusterIDs) == 0 {
			slog.Warn("no clusters matched, returning empty device list", "func", "GetDevices", "clusters", filters.Clusters)
			return &netbox.PaginatedDeviceWithConfigContextList{}, nil
		}
		req = req.ClusterId(clusterIDs)
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

// resolveClusterIDs convertit des noms de clusters NetBox en IDs.
// L'API DCIM filtre par ClusterId (int32), pas par nom.
func resolveClusterIDs(ctx *context.Context, client *netbox.APIClient, clusterNames []string) ([]*int32, error) {
	result, _, err := client.VirtualizationAPI.
		VirtualizationClustersList(*ctx).
		Name(clusterNames).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot resolve cluster names to IDs: %w", err)
	}

	ids := make([]*int32, 0, len(result.Results))
	for i := range result.Results {
		id := result.Results[i].Id
		ids = append(ids, &id)
	}
	return ids, nil
}

func GetSystemDiskInventoryItems(ctx *context.Context, client *netbox.APIClient, deviceID int32) ([]netbox.InventoryItem, error) {
	result, _, err := client.DcimAPI.DcimInventoryItemsList(*ctx).
		DeviceId([]int32{deviceID}).
		Role([]string{"system-disk"}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list inventory items for device %d: %w", deviceID, err)
	}
	return result.Results, nil
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

// GetTaggedAddressesFromPrefix returns IP addresses tagged with tag in the
// given prefix. It reuses an already-fetched prefix instead of re-querying it.
func GetTaggedAddressesFromPrefix(ctx *context.Context, client *netbox.APIClient, tag string, prefix *netbox.Prefix) (result []netbox.IPAddress) {
	addresses, _, err := client.IpamAPI.IpamIpAddressesList(*ctx).Parent([]string{prefix.Display}).Tag([]string{tag}).Execute()
	if err != nil {
		slog.Warn("failed to list addresses by tag", "func", "GetTaggedAddressesFromPrefix", "prefix", prefix.Display, "tag", tag, "error", err.Error())
		return
	}
	if addresses.Count == 0 {
		slog.Warn("no address found with tag", "func", "GetTaggedAddressesFromPrefix", "prefix", prefix.Display, "tag", tag)
	}
	for _, address := range addresses.Results {
		result = append(result, address)
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
