/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package talos_test

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

	v1alpha1 "github.com/siderolabs/talos/pkg/machinery/config/types/v1alpha1"

	"machinecfg/pkg/common"
	"machinecfg/pkg/talos"
)

const (
	talosTestDeviceID    = 1
	talosTestInterfaceID = 100
	talosTestIPAddress   = "10.0.0.1/24"
	talosTestPrefixDisp  = "10.0.0.0/24"
	talosTestHostname    = "talos-test-01"
)

func talosTestDeviceJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      talosTestDeviceID,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": talosTestHostname,
		"name":    talosTestHostname,
		"serial":  "SN001",
		"device_type": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/device-types/1/", "display": "test-type",
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
			"display": "test-site", "name": "test-site", "slug": "test-site",
		},
		"tenant": map[string]any{
			"id": 1, "url": "http://localhost/api/tenancy/tenants/1/",
			"display": "testing", "name": "testing", "slug": "testing",
		},
		"location":                  nil,
		"rack":                      nil,
		"status":                    map[string]string{"value": "active", "label": "Active"},
		"console_port_count":        0,
		"console_server_port_count": 0,
		"power_port_count":          0,
		"power_outlet_count":        0,
		"front_port_count":          0,
		"rear_port_count":           0,
		"device_bay_count":          0,
		"module_bay_count":          0,
		"inventory_item_count":      0,
	})
	return b
}

func talosTestInterfaceListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":      talosTestInterfaceID,
			"url":     "http://localhost/api/dcim/interfaces/100/",
			"display": "eth0",
			"device": map[string]any{
				"id": talosTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
				"display": talosTestHostname,
			},
			"name":                          "eth0",
			"type":                          map[string]any{},
			"tags":                          []any{},
			"tagged_vlans":                  []any{},
			"link_peers":                    []any{},
			"connected_endpoints_reachable": false,
			"count_ipaddresses":             1,
			"count_fhrp_groups":             0,
			"_occupied":                     false,
		}},
	})
	return b
}

func talosTestIPListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":                   10,
			"url":                  "http://localhost/api/ipam/ip-addresses/10/",
			"display":              talosTestIPAddress,
			"address":              talosTestIPAddress,
			"family":               map[string]any{"value": 4, "label": "IPv4"},
			"assigned_object_id":   talosTestInterfaceID,
			"assigned_object_type": "dcim.interface",
			"nat_outside":          []any{},
			"status":               map[string]string{"value": "active", "label": "Active"},
			"tags":                 []any{},
			"custom_fields":        map[string]any{},
		}},
	})
	return b
}

func talosTestEmptyIPListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 0, "next": nil, "previous": nil,
		"results": []any{},
	})
	return b
}

func talosTestPrefixListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":       1,
			"url":      "http://localhost/api/ipam/prefixes/1/",
			"display":  talosTestPrefixDisp,
			"prefix":   talosTestPrefixDisp,
			"family":   map[string]any{"value": 4, "label": "IPv4"},
			"vlan":     nil,
			"children": 0,
			"_depth":   0,
			"status":   map[string]string{"value": "active", "label": "Active"},
		}},
	})
	return b
}

// newTalosTestServer starts an httptest.Server that mocks all NetBox API
// endpoints called during CreateTalosConfigs execution.
func newTalosTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{talosTestDeviceJSON()},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		path := r.URL.Path
		q := r.URL.Query()

		switch {
		case strings.HasSuffix(path, "/api/dcim/devices/"):
			_, _ = w.Write(deviceListJSON)

		case strings.HasSuffix(path, "/api/dcim/interfaces/"):
			_, _ = w.Write(talosTestInterfaceListJSON())

		case strings.HasSuffix(path, "/api/ipam/ip-addresses/"):
			if q.Get("interface_id") != "" {
				_, _ = w.Write(talosTestIPListJSON())
			} else {
				_, _ = w.Write(talosTestEmptyIPListJSON())
			}

		case strings.HasSuffix(path, "/api/ipam/prefixes/"):
			_, _ = w.Write(talosTestPrefixListJSON())

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))

	t.Cleanup(srv.Close)
	return srv
}

// TestCreateTalosConfigs_SimpleDevice verifies that an active device with a single
// interface produces one Talos config with the expected network interface and CIDR.
func TestCreateTalosConfigs_SimpleDevice(t *testing.T) {
	srv := newTalosTestServer(t)
	nb := netbox.NewAPIClientFor(srv.URL, "fake-token-0000000000000000000000000000000")

	filters := common.DeviceFilters{
		Sites: []string{},
		Roles: []string{},
	}

	configs, err := talos.CreateTalosConfigs(nb, context.Background(), filters)
	require.NoError(t, err)
	require.Len(t, configs, 1)

	cfg := configs[0]
	assert.Equal(t, talosTestHostname, cfg.Hostname)

	require.Len(t, cfg.Config, 1)
	v1cfg, ok := cfg.Config[0].(*v1alpha1.Config)
	require.True(t, ok, "config document should be *v1alpha1.Config")

	network := v1cfg.MachineConfig.MachineNetwork
	require.NotNil(t, network)
	assert.Equal(t, talosTestHostname, network.NetworkHostname)

	require.Len(t, network.NetworkInterfaces, 1)
	iface := network.NetworkInterfaces[0]
	assert.Equal(t, "eth0", iface.DeviceInterface)
	assert.Equal(t, talosTestIPAddress, iface.DeviceCIDR)
}
