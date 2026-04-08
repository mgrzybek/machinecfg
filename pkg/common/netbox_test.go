/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package common_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"machinecfg/pkg/common"
)

const (
	gdTestClusterID   = int32(42)
	gdTestClusterName = "cluster-0"
	gdTestDeviceID    = int32(1)
	gdTestDeviceIP    = "10.0.0.1/24"
)

// gdDeviceJSON builds a minimal DeviceWithConfigContext JSON payload.
func gdDeviceJSON(id int32) map[string]any {
	return map[string]any{
		"id":      id,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": "test-device",
		"name":    "test-device",
		"serial":  "",
		"device_type": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/device-types/1/",
			"display": "test-type",
			"manufacturer": map[string]any{
				"id": 1, "url": "http://localhost/api/dcim/manufacturers/1/",
				"display": "mfr", "name": "mfr", "slug": "mfr",
			},
			"model": "test-model", "slug": "test-model",
			"console_port_template_count":        0,
			"console_server_port_template_count": 0,
			"power_port_template_count":          0,
			"power_outlet_template_count":        0,
			"interface_template_count":           0,
			"front_port_template_count":          0,
			"rear_port_template_count":           0,
			"device_bay_template_count":          0,
			"module_bay_template_count":          0,
			"inventory_item_template_count":      0,
		},
		"role": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/device-roles/1/",
			"display": "worker", "name": "worker", "slug": "worker", "_depth": 0,
		},
		"site": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/sites/1/",
			"display": "site-a", "name": "site-a", "slug": "site-a",
		},
		"primary_ip4": map[string]any{
			"id": 10, "url": "http://localhost/api/ipam/ip-addresses/10/",
			"display": gdTestDeviceIP, "address": gdTestDeviceIP,
			"family": map[string]any{"value": 4, "label": "IPv4"},
		},
		"status":                    map[string]string{"value": "staged", "label": "Staged"},
		"console_port_count":        0,
		"console_server_port_count": 0,
		"power_port_count":          0,
		"power_outlet_count":        0,
		"front_port_count":          0,
		"rear_port_count":           0,
		"device_bay_count":          0,
		"module_bay_count":          0,
		"inventory_item_count":      0,
	}
}

// gdPaginatedDeviceListJSON wraps devices in a PaginatedDeviceWithConfigContextList payload.
func gdPaginatedDeviceListJSON(devices ...map[string]any) []byte {
	results := make([]any, len(devices))
	for i, d := range devices {
		results[i] = d
	}
	b, _ := json.Marshal(map[string]any{
		"count": len(results), "next": nil, "previous": nil,
		"results": results,
	})
	return b
}

// gdClusterListJSON returns a PaginatedClusterList with one cluster entry.
func gdClusterListJSON(id int32, name string) []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":      id,
			"url":     "http://localhost/api/virtualization/clusters/42/",
			"display": name,
			"name":    name,
			"type": map[string]any{
				"id": 1, "url": "http://localhost/api/virtualization/cluster-types/1/",
				"display": "managed-kubernetes", "name": "managed-kubernetes", "slug": "managed-kubernetes",
			},
			"status": map[string]string{"value": "active", "label": "Active"},
		}},
	})
	return b
}

// gdEmptyClusterListJSON returns an empty PaginatedClusterList.
func gdEmptyClusterListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 0, "next": nil, "previous": nil,
		"results": []any{},
	})
	return b
}

// newGetDevicesTestServer starts a mock NetBox server for GetDevices tests.
//
// Behaviour:
//   - GET /api/virtualization/clusters/?name=cluster-0  → one cluster (ID 42)
//   - GET /api/virtualization/clusters/?name=<other>    → empty list
//   - GET /api/dcim/devices/?cluster_id=42             → one device
//   - GET /api/dcim/devices/ (no cluster_id)           → two devices
func newGetDevicesTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	oneDevice := gdPaginatedDeviceListJSON(gdDeviceJSON(gdTestDeviceID))
	twoDevices := gdPaginatedDeviceListJSON(gdDeviceJSON(gdTestDeviceID), gdDeviceJSON(gdTestDeviceID+1))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		path := r.URL.Path
		q := r.URL.Query()

		switch {
		case strings.HasSuffix(path, "/api/virtualization/clusters/"):
			name := q.Get("name")
			if name == gdTestClusterName {
				_, _ = w.Write(gdClusterListJSON(gdTestClusterID, gdTestClusterName))
			} else {
				_, _ = w.Write(gdEmptyClusterListJSON())
			}

		case strings.HasSuffix(path, "/api/dcim/devices/"):
			if q.Get("cluster_id") != "" {
				_, _ = w.Write(oneDevice)
			} else {
				_, _ = w.Write(twoDevices)
			}

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))

	t.Cleanup(srv.Close)
	return srv
}

func TestGetDevices_ClusterFilter(t *testing.T) {
	tests := []struct {
		name      string
		clusters  []string
		wantCount int32
	}{
		{
			name:      "no cluster filter — all devices returned",
			clusters:  []string{},
			wantCount: 2,
		},
		{
			name:      "existing cluster — only cluster devices returned",
			clusters:  []string{gdTestClusterName},
			wantCount: 1,
		},
		{
			name:      "nonexistent cluster — empty list",
			clusters:  []string{"nonexistent"},
			wantCount: 0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newGetDevicesTestServer(t)
			nb := netbox.NewAPIClientFor(srv.URL, "fake-token-0000000000000000000000000000000")

			ctx := context.Background()
			filters := common.DeviceFilters{
				Clusters: tc.clusters,
			}

			devices, err := common.GetDevices(&ctx, nb, filters)
			require.NoError(t, err)
			assert.Equal(t, tc.wantCount, devices.Count)
		})
	}
}
