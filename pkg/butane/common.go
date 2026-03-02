/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package butane

import (
	"fmt"

	"github.com/netbox-community/go-netbox/v4"
)

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
