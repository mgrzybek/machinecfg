//go:build integration

/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/

package butane_test

import (
	"context"
	"os"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"machinecfg/pkg/butane"
	"machinecfg/pkg/common"
)

// skipIfMissingCreds skips the test when NetBox env vars are absent.
func skipIfMissingCreds(t *testing.T) (endpoint, token string) {
	t.Helper()
	token = os.Getenv("NETBOX_TOKEN")
	endpoint = os.Getenv("NETBOX_ENDPOINT")
	if token == "" || endpoint == "" {
		t.Skip("NETBOX_TOKEN and NETBOX_ENDPOINT are required for integration tests")
	}
	return endpoint, token
}

// TestCreateFlatcars_Integration verifies that CreateFlatcars produces at least
// one Flatcar config for staged devices in the "testing" tenant, and that each
// config contains the mandatory /etc/hostname and /etc/dcim.yaml files.
//
// Prerequisites:
//   - NETBOX_TOKEN and NETBOX_ENDPOINT must be set.
//   - The "testing" tenant must exist in NetBox with at least one staged device
//     that has an interface with at least one IP address (run task seed-netbox).
func TestCreateFlatcars_Integration(t *testing.T) {
	endpoint, token := skipIfMissingCreds(t)

	ctx := context.Background()
	nb := netbox.NewAPIClientFor(endpoint, token)

	// Verify the testing tenant exists.
	tenantResp, _, err := nb.TenancyAPI.TenancyTenantsList(ctx).Slug([]string{"testing"}).Execute()
	require.NoError(t, err, "failed to list tenants")
	require.NotEmpty(t, tenantResp.Results, "testing tenant not found in NetBox — run task seed-netbox first")

	filters := common.DeviceFilters{
		Tenants: []string{"testing"},
		Sites:   []string{},
		Roles:   []string{},
	}

	// Verify there are staged devices to process.
	stagedFilters := filters
	stagedFilters.Status = []string{"staged"}
	devices, devErr := common.GetDevices(&ctx, nb, stagedFilters)
	require.NoError(t, devErr)
	if devices.Count == 0 {
		t.Skip("no staged devices in the testing tenant — run task seed-netbox first")
	}

	flatcars, err := butane.CreateFlatcars(nb, ctx, filters)
	// Per-device errors are joined; don't fail on partial failures from
	// intentionally incomplete devices (test-missing-ip-01, test-missing-mac-01).
	if err != nil {
		t.Logf("CreateFlatcars returned partial errors (expected for error-path devices): %v", err)
	}

	require.NotEmpty(t, flatcars, "at least one Flatcar config should be generated")

	for _, fc := range flatcars {
		fc := fc
		t.Run(fc.Hostname, func(t *testing.T) {
			assert.NotEmpty(t, fc.Hostname, "Hostname must not be empty")

			paths := make(map[string]bool, len(fc.Config.Storage.Files))
			for _, f := range fc.Config.Storage.Files {
				paths[f.Path] = true
			}
			assert.True(t, paths["/etc/hostname"], "missing /etc/hostname in config for %s", fc.Hostname)
			assert.True(t, paths["/etc/dcim.yaml"], "missing /etc/dcim.yaml in config for %s", fc.Hostname)
		})
	}
}
