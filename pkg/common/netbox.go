package common

import (
	"context"
	"log/slog"
	"net/http"

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
