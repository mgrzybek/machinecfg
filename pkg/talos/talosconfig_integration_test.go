//go:build integration

/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/

package talos_test

import (
	"context"
	"os"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	v1alpha1 "github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"machinecfg/pkg/common"
	"machinecfg/pkg/talos"
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

// TestCreateTalosConfigs_Integration verifies that CreateTalosConfigs produces
// at least one Talos config for active devices in the "testing" tenant, and that
// each config contains a non-empty hostname and a valid v1alpha1 machine config.
//
// Prerequisites:
//   - NETBOX_TOKEN and NETBOX_ENDPOINT must be set.
//   - The "testing" tenant must exist in NetBox with at least one active device
//     that has an interface with at least one IP address.
//
// Note: the seed-netbox tool creates devices in "staged" status. To exercise
// this test, at least one device in the testing tenant must be manually set to
// "active", or the seed must be extended to include active devices.
func TestCreateTalosConfigs_Integration(t *testing.T) {
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

	// Verify there are active devices to process.
	activeFilters := filters
	activeFilters.Status = []string{"active"}
	devices, devErr := common.GetDevices(&ctx, nb, activeFilters)
	require.NoError(t, devErr)
	if devices.Count == 0 {
		t.Skip("no active devices in the testing tenant — set at least one device to active status")
	}

	configs, err := talos.CreateTalosConfigs(nb, ctx, filters)
	if err != nil {
		t.Logf("CreateTalosConfigs returned partial errors: %v", err)
	}

	require.NotEmpty(t, configs, "at least one Talos config should be generated")

	for _, cfg := range configs {
		cfg := cfg
		t.Run(cfg.Hostname, func(t *testing.T) {
			assert.NotEmpty(t, cfg.Hostname, "Hostname must not be empty")
			require.Len(t, cfg.Config, 1, "exactly one config document should be generated")

			v1cfg, ok := cfg.Config[0].(*v1alpha1.Config)
			require.True(t, ok, "config document should be *v1alpha1.Config")
			require.NotNil(t, v1cfg.MachineConfig)
			require.NotNil(t, v1cfg.MachineConfig.MachineNetwork)
			assert.NotEmpty(t, v1cfg.MachineConfig.MachineNetwork.NetworkHostname,
				"MachineNetwork.NetworkHostname must not be empty")
		})
	}
}
