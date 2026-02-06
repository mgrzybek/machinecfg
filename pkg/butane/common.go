package butane

import (
	"fmt"
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
