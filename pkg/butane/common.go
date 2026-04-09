/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package butane

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/netbox-community/go-netbox/v4"
)

// getHostname returns the DNS name of the device's primary IP address.
// Falls back to the device name if no primary IP or DNS name is set.
func getHostname(ctx context.Context, c *netbox.APIClient, device *netbox.DeviceWithConfigContext) string {
	fallback := device.Name.Get()

	var primaryIPID *int32
	if ip := device.PrimaryIp4.Get(); ip != nil {
		primaryIPID = &ip.Id
	} else if ip := device.PrimaryIp.Get(); ip != nil {
		primaryIPID = &ip.Id
	}

	if primaryIPID == nil {
		return *fallback
	}

	ipAddr, _, err := c.IpamAPI.IpamIpAddressesRetrieve(ctx, *primaryIPID).Execute()
	if err != nil {
		slog.Warn("failed to retrieve primary IP address", "id", *primaryIPID, "error", err.Error())
		return *fallback
	}

	if ipAddr.DnsName != nil && *ipAddr.DnsName != "" {
		return *ipAddr.DnsName
	}

	return *fallback
}

func createDCIMFile(device *netbox.DeviceWithConfigContext) string {
	var location, rack string
	if l := device.Location.Get(); l != nil {
		location = l.GetName()
	}
	if r := device.Rack.Get(); r != nil {
		rack = r.GetName()
	}
	return fmt.Sprintf(
		"---\ngenerated-by: machinecfg\nserial: %s\nmodel: %s\nrole: %s\nsite: %s\nlocation: %s\nracks: %s\ntenant: %s\nstatus: %s",
		*device.Serial,
		device.DeviceType.Slug,
		device.Role.GetName(),
		device.Site.GetName(),
		location,
		rack,
		device.Tenant.Get().GetName(),
		device.Status.GetLabel(),
	)
}
