/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package butane_test

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

	"machinecfg/pkg/butane"
	"machinecfg/pkg/common"
)

const (
	flatcarTestDeviceID    = 1
	flatcarTestIPID        = int32(10)
	flatcarTestInterfaceID = 100
	flatcarTestIPAddress   = "10.0.0.1/24"
	flatcarTestMACAddress  = "aa:bb:cc:dd:ee:ff"
	flatcarTestPrefixDisp  = "10.0.0.0/24"
	flatcarTestHostname    = "flatcar-test-01"
)

func flatcarTestDeviceJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      flatcarTestDeviceID,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": flatcarTestHostname,
		"name":    flatcarTestHostname,
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
		"primary_ip4": map[string]any{
			"id": flatcarTestIPID, "url": "http://localhost/api/ipam/ip-addresses/10/",
			"display": flatcarTestIPAddress, "address": flatcarTestIPAddress,
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
	})
	return b
}

func flatcarTestInterfaceListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":      flatcarTestInterfaceID,
			"url":     "http://localhost/api/dcim/interfaces/100/",
			"display": "eth0",
			"device": map[string]any{
				"id": flatcarTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
				"display": flatcarTestHostname,
			},
			"name":                          "eth0",
			"type":                          map[string]any{},
			"tags":                          []any{},
			"link_peers":                    []any{},
			"tagged_vlans":                  []any{},
			"connected_endpoints_reachable": false,
			"count_ipaddresses":             1,
			"count_fhrp_groups":             0,
			"_occupied":                     false,
			"mac_address":                   flatcarTestMACAddress,
		}},
	})
	return b
}

func flatcarTestIPListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":                   flatcarTestIPID,
			"url":                  "http://localhost/api/ipam/ip-addresses/10/",
			"display":              flatcarTestIPAddress,
			"address":              flatcarTestIPAddress,
			"family":               map[string]any{"value": 4, "label": "IPv4"},
			"assigned_object_id":   flatcarTestInterfaceID,
			"assigned_object_type": "dcim.interface",
			"nat_outside":          []any{},
			"status":               map[string]string{"value": "active", "label": "Active"},
			"tags":                 []any{},
			"custom_fields":        map[string]any{},
		}},
	})
	return b
}

func flatcarTestIPRetrieveJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":            flatcarTestIPID,
		"url":           "http://localhost/api/ipam/ip-addresses/10/",
		"display":       flatcarTestIPAddress,
		"address":       flatcarTestIPAddress,
		"dns_name":      nil,
		"family":        map[string]any{"value": 4, "label": "IPv4"},
		"nat_outside":   []any{},
		"status":        map[string]string{"value": "active", "label": "Active"},
		"tags":          []any{},
		"custom_fields": map[string]any{},
	})
	return b
}

func flatcarTestEmptyIPListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 0, "next": nil, "previous": nil,
		"results": []any{},
	})
	return b
}

func flatcarTestPrefixListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":       1,
			"url":      "http://localhost/api/ipam/prefixes/1/",
			"display":  flatcarTestPrefixDisp,
			"prefix":   flatcarTestPrefixDisp,
			"family":   map[string]any{"value": 4, "label": "IPv4"},
			"vlan":     nil,
			"children": 0,
			"_depth":   0,
			"status":   map[string]string{"value": "active", "label": "Active"},
		}},
	})
	return b
}

// newFlatcarTestServer starts an httptest.Server that mocks all NetBox API
// endpoints called during CreateFlatcars execution.
func newFlatcarTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{flatcarTestDeviceJSON()},
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
			_, _ = w.Write(flatcarTestInterfaceListJSON())

		case strings.HasSuffix(path, "/api/ipam/ip-addresses/"):
			if q.Get("interface_id") != "" {
				_, _ = w.Write(flatcarTestIPListJSON())
			} else {
				// gateway/dns tag queries or any other list query → empty
				_, _ = w.Write(flatcarTestEmptyIPListJSON())
			}

		case strings.Contains(path, "/api/ipam/ip-addresses/"):
			// retrieve by ID: /api/ipam/ip-addresses/10/
			_, _ = w.Write(flatcarTestIPRetrieveJSON())

		case strings.HasSuffix(path, "/api/ipam/prefixes/"):
			_, _ = w.Write(flatcarTestPrefixListJSON())

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))

	t.Cleanup(srv.Close)
	return srv
}

// flatcarTestVLANInterfaceListJSON returns an interface whose tagged_vlans
// contains VLAN 100, triggering the VLAN branch in extractFlatcarData.
func flatcarTestVLANInterfaceListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":      flatcarTestInterfaceID,
			"url":     "http://localhost/api/dcim/interfaces/100/",
			"display": "eth0",
			"device": map[string]any{
				"id": flatcarTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
				"display": flatcarTestHostname,
			},
			"name": "eth0",
			"type": map[string]any{},
			"tags": []any{},
			"tagged_vlans": []any{map[string]any{
				"id": 1, "url": "http://localhost/api/ipam/vlans/1/",
				"display": "vlan100", "vid": 100, "name": "vlan100",
			}},
			"link_peers":                    []any{},
			"connected_endpoints_reachable": false,
			"count_ipaddresses":             1,
			"count_fhrp_groups":             0,
			"_occupied":                     false,
			"mac_address":                   flatcarTestMACAddress,
		}},
	})
	return b
}

// flatcarTestVLANPrefixListJSON returns a prefix whose vlan field points to
// VLAN 100, so that IsVlanIDInVlanList returns true and the VLAN branch is taken.
func flatcarTestVLANPrefixListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":      2,
			"url":     "http://localhost/api/ipam/prefixes/2/",
			"display": "10.0.1.0/24",
			"prefix":  "10.0.1.0/24",
			"family":  map[string]any{"value": 4, "label": "IPv4"},
			"vlan": map[string]any{
				"id": 1, "url": "http://localhost/api/ipam/vlans/1/",
				"display": "vlan100", "vid": 100, "name": "vlan100",
			},
			"children": 0,
			"_depth":   0,
			"status":   map[string]string{"value": "active", "label": "Active"},
		}},
	})
	return b
}

// newFlatcarVLANTestServer starts an httptest.Server for the VLAN test scenario.
// The interface has tagged_vlans={vid:100} and the prefix carries the matching vlan.
func newFlatcarVLANTestServer(t *testing.T) *httptest.Server {
	t.Helper()

	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{flatcarTestDeviceJSON()},
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
			_, _ = w.Write(flatcarTestVLANInterfaceListJSON())

		case strings.HasSuffix(path, "/api/ipam/ip-addresses/"):
			if q.Get("interface_id") != "" {
				_, _ = w.Write(flatcarTestIPListJSON())
			} else {
				_, _ = w.Write(flatcarTestEmptyIPListJSON())
			}

		case strings.Contains(path, "/api/ipam/ip-addresses/"):
			_, _ = w.Write(flatcarTestIPRetrieveJSON())

		case strings.HasSuffix(path, "/api/ipam/prefixes/"):
			_, _ = w.Write(flatcarTestVLANPrefixListJSON())

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))

	t.Cleanup(srv.Close)
	return srv
}

// TestCreateFlatcars_SimpleDevice verifies that a staged device with a single
// interface produces one Flatcar config containing the expected systemd-networkd
// and hostname files.
func TestCreateFlatcars_SimpleDevice(t *testing.T) {
	srv := newFlatcarTestServer(t)
	nb := netbox.NewAPIClientFor(srv.URL, "fake-token-0000000000000000000000000000000")

	filters := common.DeviceFilters{
		Sites: []string{},
		Roles: []string{},
	}

	flatcars, err := butane.CreateFlatcars(nb, context.Background(), filters)
	require.NoError(t, err)
	require.Len(t, flatcars, 1)

	fc := flatcars[0]
	assert.Equal(t, flatcarTestHostname, fc.Hostname)

	// Collect all file paths from the generated config
	paths := make([]string, 0, len(fc.Config.Storage.Files))
	for _, f := range fc.Config.Storage.Files {
		paths = append(paths, f.Path)
	}

	assert.Contains(t, paths, "/etc/systemd/network/01-eth0.network",
		"systemd-networkd file for eth0 should be generated")
	assert.Contains(t, paths, "/etc/hostname",
		"/etc/hostname file should be generated")
	assert.Contains(t, paths, "/etc/dcim.yaml",
		"/etc/dcim.yaml file should be generated")
}

// TestCreateFlatcars_VLANInterface verifies that an interface with a tagged VLAN
// produces both a .netdev file and a VLAN-specific .network file.
func TestCreateFlatcars_VLANInterface(t *testing.T) {
	srv := newFlatcarVLANTestServer(t)
	nb := netbox.NewAPIClientFor(srv.URL, "fake-token-0000000000000000000000000000000")

	filters := common.DeviceFilters{
		Sites: []string{},
		Roles: []string{},
	}

	flatcars, err := butane.CreateFlatcars(nb, context.Background(), filters)
	require.NoError(t, err)
	require.Len(t, flatcars, 1)

	paths := make([]string, 0, len(flatcars[0].Config.Storage.Files))
	for _, f := range flatcars[0].Config.Storage.Files {
		paths = append(paths, f.Path)
	}

	assert.Contains(t, paths, "/etc/systemd/network/00-vlan100.netdev",
		".netdev file for VLAN 100 should be generated")
	assert.Contains(t, paths, "/etc/systemd/network/01-vlan100.network",
		".network file for VLAN 100 should be generated")
}
