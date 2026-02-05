package butane

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/netbox-community/go-netbox/v4"
)

func hasDHCPTag(tags []netbox.NestedTag) (answer bool) {
	for _, tag := range tags {
		if strings.ToLower(tag.GetName()) == "dhcp" {
			answer = true
		}
	}

	return answer
}

func getTaggedAddressesFromPrefixOfAddr(ctx *context.Context, client *netbox.APIClient, tag string, addr *netbox.IPAddress) (result []netbox.IPAddress) {
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

func isVlanIDinVlanList(vlanID int32, vlans []netbox.VLAN) (result bool) {
	for _, v := range vlans {
		if v.Vid == vlanID {
			result = true
		}
	}

	return result
}

func createDCIMFile(device *netbox.DeviceWithConfigContext) string {
	return fmt.Sprintf(
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
}
