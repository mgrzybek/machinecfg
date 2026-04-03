//go:build integration

/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/

package tinkerbell_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"machinecfg/pkg/common"
	"machinecfg/pkg/tinkerbell"
)

// TestCreateHardwares_SystemDiskIntegration verifies that CreateHardwares
// produces Hardware objects whose spec.disks reflect the system-disk inventory
// items defined in the real NetBox testing tenant.
//
// Prerequisites:
//   - NETBOX_TOKEN and NETBOX_ENDPOINT must be set.
//   - The "testing" tenant must exist in NetBox.
//   - At least one staged device in the "testing" tenant must have a primary
//     IPv4, a primary interface with a MAC address, and at least one inventory
//     item with role slug "system-disk".
func TestCreateHardwares_SystemDiskIntegration(t *testing.T) {
	endpoint, token := skipIfMissingCreds(t)

	ctx := context.Background()
	nb := netbox.NewAPIClientFor(endpoint, token)

	// Verify the testing tenant exists.
	tenantResp, _, err := nb.TenancyAPI.TenancyTenantsList(ctx).Slug([]string{"testing"}).Execute()
	require.NoError(t, err, "failed to list tenants")
	require.NotEmpty(t, tenantResp.Results, "testing tenant not found in NetBox — create it first")

	filters := common.DeviceFilters{
		Tenants: []string{"testing"},
		Sites:   []string{},
		Roles:   []string{},
	}

	// Check that there are staged devices to process.
	stagedFilters := filters
	stagedFilters.Status = []string{"staged"}
	devices, devErr := common.GetDevices(&ctx, nb, stagedFilters)
	require.NoError(t, devErr)

	if devices.Count == 0 {
		t.Skip("no staged devices with primary IP in the testing tenant — skipping")
	}

	hardwares, err := tinkerbell.CreateHardwares(nb, ctx, filters, nil)
	// CreateHardwares logs per-device errors but returns nil overall error.
	require.NoError(t, err)

	// For each generated Hardware, verify that spec.disks matches the
	// system-disk inventory items of the corresponding NetBox device.
	for _, hw := range hardwares {
		hw := hw

		deviceIDStr, ok := hw.Labels["netbox-device-id"]
		require.True(t, ok, "Hardware %s is missing netbox-device-id label", hw.Name)

		deviceID64, parseErr := strconv.ParseInt(deviceIDStr, 10, 32)
		require.NoError(t, parseErr, "invalid netbox-device-id %q on %s", deviceIDStr, hw.Name)
		deviceID := int32(deviceID64)

		// Fetch the expected system-disk inventory items for this device.
		items, itemErr := nb.DcimAPI.DcimInventoryItemsList(ctx).
			DeviceId([]int32{deviceID}).
			Role([]string{"system-disk"}).
			Execute()
		require.NoError(t, itemErr)

		expectedPaths := make([]string, 0, len(items.Results))
		for _, item := range items.Results {
			expectedPaths = append(expectedPaths, item.Name)
		}

		actualPaths := make([]string, 0, len(hw.Spec.Disks))
		for _, d := range hw.Spec.Disks {
			actualPaths = append(actualPaths, d.Device)
		}

		assert.ElementsMatch(t, expectedPaths, actualPaths,
			"spec.disks mismatch for Hardware %s (device_id=%d)", hw.Name, deviceID)
	}
}
