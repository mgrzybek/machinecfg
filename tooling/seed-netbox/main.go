/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later

seed-netbox creates the "testing" tenant and its associated devices in a NetBox
instance, so that the integration test environment can be reproduced from scratch.

Usage:

	NETBOX_ENDPOINT=https://netbox.example.com NETBOX_TOKEN=<token> go run ./tooling/seed-netbox/
*/
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/netbox-community/go-netbox/v4"
)

// networkSpec describes the network configuration to seed for a device.
type networkSpec struct {
	ifaceName string
	macAddr   string // empty = no MAC (test-missing-mac-01)
	ipAddress string // empty = no IP (test-missing-ip-01)
	ifaceType netbox.InterfaceTypeValue
}

const (
	tenantName       = "testing"
	tenantSlug       = "testing"
	siteName         = "testing-site"
	siteSlug         = "testing-site"
	roleName         = "worker"
	roleSlug         = "worker"
	manufacturerName = "test-manufacturer"
	manufacturerSlug = "test-manufacturer"
	deviceTypeName   = "test-device-type"
	deviceTypeSlug   = "test-device-type"
)

// deviceSpec describes a device to seed.
type deviceSpec struct {
	name    string
	status  string // staged, active, planned, offline
	network *networkSpec
}

var devicesToSeed = []deviceSpec{
	{
		name: "test-simple-01", status: "staged",
		network: &networkSpec{ifaceName: "eth0", macAddr: "aa:bb:cc:00:01:01", ipAddress: "10.10.0.1/24", ifaceType: netbox.INTERFACETYPEVALUE__1000BASE_T},
	},
	{
		name: "test-bond-01", status: "staged",
		network: &networkSpec{ifaceName: "bond0", macAddr: "aa:bb:cc:00:02:01", ipAddress: "10.10.0.2/24", ifaceType: netbox.INTERFACETYPEVALUE_LAG},
	},
	{
		name: "test-vlan-01", status: "staged",
		network: &networkSpec{ifaceName: "eth0", macAddr: "aa:bb:cc:00:03:01", ipAddress: "10.10.1.1/24", ifaceType: netbox.INTERFACETYPEVALUE__1000BASE_T},
	},
	{
		name: "test-bond-vlan-01", status: "staged",
		network: &networkSpec{ifaceName: "bond0", macAddr: "aa:bb:cc:00:04:01", ipAddress: "10.10.1.2/24", ifaceType: netbox.INTERFACETYPEVALUE_LAG},
	},
	{
		name: "test-missing-ip-01", status: "staged",
		network: &networkSpec{ifaceName: "eth0", macAddr: "aa:bb:cc:00:05:01", ipAddress: "", ifaceType: netbox.INTERFACETYPEVALUE__1000BASE_T},
	},
	{
		name: "test-missing-mac-01", status: "staged",
		network: &networkSpec{ifaceName: "eth0", macAddr: "", ipAddress: "10.10.0.5/24", ifaceType: netbox.INTERFACETYPEVALUE__1000BASE_T},
	},
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	endpoint := os.Getenv("NETBOX_ENDPOINT")
	token := os.Getenv("NETBOX_TOKEN")

	if endpoint == "" {
		fmt.Fprintln(os.Stderr, "Error: NETBOX_ENDPOINT is required")
		os.Exit(1)
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "Error: NETBOX_TOKEN is required")
		os.Exit(1)
	}

	ctx := context.Background()
	client := netbox.NewAPIClientFor(endpoint, token)

	tenantID, err := ensureTenant(ctx, client)
	if err != nil {
		slog.Error("failed to ensure tenant", "error", err)
		os.Exit(1)
	}
	slog.Info("tenant ready", "name", tenantName, "id", tenantID)

	siteID, err := ensureSite(ctx, client, tenantID)
	if err != nil {
		slog.Error("failed to ensure site", "error", err)
		os.Exit(1)
	}
	slog.Info("site ready", "name", siteName, "id", siteID)

	roleID, err := ensureDeviceRole(ctx, client)
	if err != nil {
		slog.Error("failed to ensure device role", "error", err)
		os.Exit(1)
	}
	slog.Info("device role ready", "name", roleName, "id", roleID)

	manufacturerID, err := ensureManufacturer(ctx, client)
	if err != nil {
		slog.Error("failed to ensure manufacturer", "error", err)
		os.Exit(1)
	}
	slog.Info("manufacturer ready", "name", manufacturerName, "id", manufacturerID)

	deviceTypeID, err := ensureDeviceType(ctx, client, manufacturerID)
	if err != nil {
		slog.Error("failed to ensure device type", "error", err)
		os.Exit(1)
	}
	slog.Info("device type ready", "name", deviceTypeName, "id", deviceTypeID)

	// Seed prefixes so that extractFlatcarData can resolve network gateways.
	for _, prefix := range []string{"10.10.0.0/24", "10.10.1.0/24"} {
		if err := ensurePrefix(ctx, client, prefix); err != nil {
			slog.Error("failed to ensure prefix", "prefix", prefix, "error", err)
			os.Exit(1)
		}
		slog.Info("prefix ready", "prefix", prefix)
	}

	for _, spec := range devicesToSeed {
		deviceID, err := ensureDevice(ctx, client, spec, siteID, roleID, deviceTypeID, tenantID)
		if err != nil {
			slog.Error("failed to ensure device", "name", spec.name, "error", err)
			os.Exit(1)
		}
		slog.Info("device ready", "name", spec.name, "id", deviceID)

		if spec.network == nil {
			continue
		}

		ifaceID, err := ensureInterface(ctx, client, deviceID, spec.network.ifaceName, spec.network.ifaceType)
		if err != nil {
			slog.Error("failed to ensure interface", "device", spec.name, "iface", spec.network.ifaceName, "error", err)
			os.Exit(1)
		}
		slog.Info("interface ready", "device", spec.name, "iface", spec.network.ifaceName, "id", ifaceID)

		if spec.network.ipAddress == "" {
			slog.Info("skipping IP assignment (test-missing-ip scenario)", "device", spec.name)
			continue
		}

		ipID, err := ensureIPAddress(ctx, client, spec.network.ipAddress, ifaceID)
		if err != nil {
			slog.Error("failed to ensure IP address", "device", spec.name, "ip", spec.network.ipAddress, "error", err)
			os.Exit(1)
		}
		slog.Info("IP address ready", "device", spec.name, "ip", spec.network.ipAddress, "id", ipID)

		if err := ensureDevicePrimaryIP(ctx, client, deviceID, ipID); err != nil {
			slog.Error("failed to set primary IP", "device", spec.name, "error", err)
			os.Exit(1)
		}
		slog.Info("primary IP set", "device", spec.name, "ip", spec.network.ipAddress)
	}

	fmt.Println("Seed complete.")
}

func ensureTenant(ctx context.Context, client *netbox.APIClient) (int32, error) {
	result, _, err := client.TenancyAPI.TenancyTenantsList(ctx).Name([]string{tenantName}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list tenants: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	req := netbox.TenantRequest{Name: tenantName, Slug: tenantSlug}
	created, _, err := client.TenancyAPI.TenancyTenantsCreate(ctx).TenantRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create tenant: %w", err)
	}
	return created.Id, nil
}

func ensureSite(ctx context.Context, client *netbox.APIClient, tenantID int32) (int32, error) {
	result, _, err := client.DcimAPI.DcimSitesList(ctx).Name([]string{siteName}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list sites: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	tenantRef := netbox.Int32AsASNRangeRequestTenant(&tenantID)
	req := netbox.WritableSiteRequest{
		Name:   siteName,
		Slug:   siteSlug,
		Tenant: netbox.NullableASNRangeRequestTenant{},
	}
	req.Tenant.Set(&tenantRef)
	created, _, err := client.DcimAPI.DcimSitesCreate(ctx).WritableSiteRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create site: %w", err)
	}
	return created.Id, nil
}

func ensureDeviceRole(ctx context.Context, client *netbox.APIClient) (int32, error) {
	result, _, err := client.DcimAPI.DcimDeviceRolesList(ctx).Name([]string{roleName}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list device roles: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	color := "000000"
	req := netbox.WritableDeviceRoleRequest{Name: roleName, Slug: roleSlug, Color: &color}
	created, _, err := client.DcimAPI.DcimDeviceRolesCreate(ctx).WritableDeviceRoleRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create device role: %w", err)
	}
	return created.Id, nil
}

func ensureManufacturer(ctx context.Context, client *netbox.APIClient) (int32, error) {
	result, _, err := client.DcimAPI.DcimManufacturersList(ctx).Name([]string{manufacturerName}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list manufacturers: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	req := netbox.ManufacturerRequest{Name: manufacturerName, Slug: manufacturerSlug}
	created, _, err := client.DcimAPI.DcimManufacturersCreate(ctx).ManufacturerRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create manufacturer: %w", err)
	}
	return created.Id, nil
}

func ensureDeviceType(ctx context.Context, client *netbox.APIClient, manufacturerID int32) (int32, error) {
	result, _, err := client.DcimAPI.DcimDeviceTypesList(ctx).Model([]string{deviceTypeName}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list device types: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	req := netbox.WritableDeviceTypeRequest{
		Manufacturer: netbox.Int32AsBriefDeviceTypeRequestManufacturer(&manufacturerID),
		Model:        deviceTypeName,
		Slug:         deviceTypeSlug,
	}
	created, _, err := client.DcimAPI.DcimDeviceTypesCreate(ctx).WritableDeviceTypeRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create device type: %w", err)
	}
	return created.Id, nil
}

// ensurePrefix creates the given prefix in NetBox if it does not already exist.
func ensurePrefix(ctx context.Context, client *netbox.APIClient, prefix string) error {
	result, _, err := client.IpamAPI.IpamPrefixesList(ctx).Prefix([]string{prefix}).Execute()
	if err != nil {
		return fmt.Errorf("list prefixes: %w", err)
	}
	if result.Count > 0 {
		return nil
	}
	req := netbox.WritablePrefixRequest{Prefix: prefix}
	_, _, err = client.IpamAPI.IpamPrefixesCreate(ctx).WritablePrefixRequest(req).Execute()
	if err != nil {
		return fmt.Errorf("create prefix %s: %w", prefix, err)
	}
	return nil
}

// ensureInterface creates a device interface in NetBox if it does not already exist.
// The MAC address is not set here — NetBox v4 requires a separate MACAddress object.
func ensureInterface(ctx context.Context, client *netbox.APIClient, deviceID int32, name string, ifaceType netbox.InterfaceTypeValue) (int32, error) {
	result, _, err := client.DcimAPI.DcimInterfacesList(ctx).DeviceId([]int32{deviceID}).Name([]string{name}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list interfaces: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	deviceRef := netbox.Int32AsBriefInterfaceRequestDevice(&deviceID)
	req := netbox.WritableInterfaceRequest{
		Device: deviceRef,
		Name:   name,
		Type:   ifaceType,
	}
	created, _, err := client.DcimAPI.DcimInterfacesCreate(ctx).WritableInterfaceRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create interface %s on device %d: %w", name, deviceID, err)
	}
	return created.Id, nil
}

// ensureIPAddress creates an IP address in NetBox and assigns it to the given
// interface if it does not already exist.
func ensureIPAddress(ctx context.Context, client *netbox.APIClient, address string, ifaceID int32) (int32, error) {
	result, _, err := client.IpamAPI.IpamIpAddressesList(ctx).Address([]string{address}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list ip addresses: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	s := "dcim.interface"
	i := int64(ifaceID)
	assignedType := *netbox.NewNullableString(&s)
	assignedID := *netbox.NewNullableInt64(&i)
	req := netbox.WritableIPAddressRequest{
		Address:            address,
		AssignedObjectType: assignedType,
		AssignedObjectId:   assignedID,
	}
	created, _, err := client.IpamAPI.IpamIpAddressesCreate(ctx).WritableIPAddressRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create ip address %s: %w", address, err)
	}
	return created.Id, nil
}

// ensureDevicePrimaryIP sets the primary IPv4 address on a device via PATCH.
func ensureDevicePrimaryIP(ctx context.Context, client *netbox.APIClient, deviceID, ipID int32) error {
	primaryIP := netbox.Int32AsDeviceWithConfigContextRequestPrimaryIp4(&ipID)
	nullablePrimary := netbox.NewNullableDeviceWithConfigContextRequestPrimaryIp4(&primaryIP)
	req := netbox.PatchedWritableDeviceWithConfigContextRequest{
		PrimaryIp4: *nullablePrimary,
	}
	_, _, err := client.DcimAPI.DcimDevicesPartialUpdate(ctx, deviceID).
		PatchedWritableDeviceWithConfigContextRequest(req).Execute()
	if err != nil {
		return fmt.Errorf("set primary IP %d on device %d: %w", ipID, deviceID, err)
	}
	return nil
}

func ensureDevice(ctx context.Context, client *netbox.APIClient, spec deviceSpec, siteID, roleID, deviceTypeID, tenantID int32) (int32, error) {
	result, _, err := client.DcimAPI.DcimDevicesList(ctx).Name([]string{spec.name}).Execute()
	if err != nil {
		return 0, fmt.Errorf("list devices: %w", err)
	}
	if result.Count > 0 {
		return result.Results[0].Id, nil
	}
	status := netbox.DeviceStatusValue(spec.status)
	tenantRef := netbox.Int32AsASNRangeRequestTenant(&tenantID)
	req := netbox.WritableDeviceWithConfigContextRequest{
		Name:       netbox.NullableString{},
		DeviceType: netbox.Int32AsDeviceBayTemplateRequestDeviceType(&deviceTypeID),
		Role:       netbox.Int32AsDeviceWithConfigContextRequestRole(&roleID),
		Site:       netbox.Int32AsDeviceWithConfigContextRequestSite(&siteID),
		Status:     &status,
		Tenant:     netbox.NullableASNRangeRequestTenant{},
	}
	req.Name.Set(&spec.name)
	req.Tenant.Set(&tenantRef)
	created, _, err := client.DcimAPI.DcimDevicesCreate(ctx).WritableDeviceWithConfigContextRequest(req).Execute()
	if err != nil {
		return 0, fmt.Errorf("create device %s: %w", spec.name, err)
	}
	return created.Id, nil
}
