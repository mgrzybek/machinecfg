/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package tinkerbell_test

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
	"machinecfg/pkg/tinkerbell"
)

const (
	hwTestDeviceID    = 1
	hwTestIPID        = 10
	hwTestInterfaceID = 100
	hwTestIPAddress   = "10.0.0.1/24"
	hwTestMACAddress  = "aa:bb:cc:dd:ee:ff"
	hwTestPrefixDisp  = "10.0.0.0/24"
)

// hardwareTestDeviceJSON returns a valid DeviceWithConfigContext JSON payload
// including primary_ip4, platform, tenant, and all other fields used by
// extractHardwareData.
func hardwareTestDeviceJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      hwTestDeviceID,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": "hw-test-01",
		"name":    "hw-test-01",
		"serial":  "SN001",
		"device_type": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/device-types/1/", "display": "test-type",
			"manufacturer": map[string]any{
				"id": 1, "url": "http://localhost/api/dcim/manufacturers/1/",
				"display": "test-mfr", "name": "test-mfr", "slug": "test-mfr",
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
			"display": "test-role", "name": "test-role", "slug": "test-role", "_depth": 0,
		},
		"site": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/sites/1/",
			"display": "test-site", "name": "test-site", "slug": "test-site",
		},
		"platform": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/platforms/1/",
			"display": "x86_64", "name": "x86_64", "slug": "x86-64",
		},
		"tenant": map[string]any{
			"id": 1, "url": "http://localhost/api/tenancy/tenants/1/",
			"display": "testing", "name": "testing", "slug": "testing",
		},
		"primary_ip": map[string]any{
			"id": hwTestIPID, "url": "http://localhost/api/ipam/ip-addresses/10/",
			"display": hwTestIPAddress, "address": hwTestIPAddress,
			"family": map[string]any{"value": 4, "label": "IPv4"},
		},
		"primary_ip4": map[string]any{
			"id": hwTestIPID, "url": "http://localhost/api/ipam/ip-addresses/10/",
			"display": hwTestIPAddress, "address": hwTestIPAddress,
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

// hardwareTestIPAddressJSON returns a PaginatedIPAddressList with one entry
// pointing to interface hwTestInterfaceID.
func hardwareTestIPAddressJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":                   hwTestIPID,
			"url":                  "http://localhost/api/ipam/ip-addresses/10/",
			"display":              hwTestIPAddress,
			"address":              hwTestIPAddress,
			"family":               map[string]any{"value": 4, "label": "IPv4"},
			"assigned_object_id":   hwTestInterfaceID,
			"assigned_object_type": "dcim.interface",
			"nat_outside":          []any{},
			"status":               map[string]string{"value": "active", "label": "Active"},
			"tags":                 []any{},
			"custom_fields":        map[string]any{},
		}},
	})
	return b
}

// hardwareTestEmptyIPListJSON returns an empty PaginatedIPAddressList.
func hardwareTestEmptyIPListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 0, "next": nil, "previous": nil,
		"results": []any{},
	})
	return b
}

// hardwareTestInterfaceJSON returns a minimal valid Interface with a MAC address.
func hardwareTestInterfaceJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      hwTestInterfaceID,
		"url":     "http://localhost/api/dcim/interfaces/100/",
		"display": "eth0",
		"device": map[string]any{
			"id": hwTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
			"display": "hw-test-01",
		},
		"name":                          "eth0",
		"type":                          map[string]any{},
		"link_peers":                    []any{},
		"connected_endpoints_reachable": false,
		"count_ipaddresses":             0,
		"count_fhrp_groups":             0,
		"_occupied":                     false,
		"primary_mac_address": map[string]any{
			"id":          1,
			"url":         "http://localhost/api/dcim/mac-addresses/1/",
			"display":     hwTestMACAddress,
			"mac_address": hwTestMACAddress,
		},
		"mac_addresses": []any{},
	})
	return b
}

// hardwareTestPrefixListJSON returns a PaginatedPrefixList with one prefix.
func hardwareTestPrefixListJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []any{map[string]any{
			"id":       1,
			"url":      "http://localhost/api/ipam/prefixes/1/",
			"display":  hwTestPrefixDisp,
			"prefix":   hwTestPrefixDisp,
			"family":   map[string]any{"value": 4, "label": "IPv4"},
			"children": 0,
			"_depth":   0,
			"status":   map[string]string{"value": "active", "label": "Active"},
		}},
	})
	return b
}

// hardwareTestInventoryItemsJSON returns a PaginatedInventoryItemList for the
// given disk device names.
func hardwareTestInventoryItemsJSON(diskNames []string) []byte {
	results := make([]any, 0, len(diskNames))
	for i, name := range diskNames {
		results = append(results, map[string]any{
			"id":      i + 1,
			"url":     "http://localhost/api/dcim/inventory-items/" + string(rune('0'+i+1)) + "/",
			"display": name,
			"device": map[string]any{
				"id": hwTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
				"display": "hw-test-01",
			},
			"name":   name,
			"_depth": 0,
		})
	}
	b, _ := json.Marshal(map[string]any{
		"count": len(results), "next": nil, "previous": nil,
		"results": results,
	})
	return b
}

// newHardwareTestServer starts an httptest.Server that mocks all NetBox API
// endpoints called during CreateHardwares execution. The systemDiskNames
// parameter controls what inventory items are returned.
func newHardwareTestServer(t *testing.T, systemDiskNames []string) *httptest.Server {
	t.Helper()

	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{hardwareTestDeviceJSON()},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		path := r.URL.Path
		q := r.URL.Query()

		switch {
		case strings.HasSuffix(path, "/api/dcim/devices/"):
			_, _ = w.Write(deviceListJSON)

		case strings.HasSuffix(path, "/api/ipam/ip-addresses/"):
			if q.Get("id") != "" {
				_, _ = w.Write(hardwareTestIPAddressJSON())
			} else {
				// tag=gateway or tag=dns queries → return empty list
				_, _ = w.Write(hardwareTestEmptyIPListJSON())
			}

		case strings.Contains(path, "/api/dcim/interfaces/"):
			_, _ = w.Write(hardwareTestInterfaceJSON())

		case strings.HasSuffix(path, "/api/ipam/prefixes/"):
			_, _ = w.Write(hardwareTestPrefixListJSON())

		case strings.HasSuffix(path, "/api/dcim/inventory-items/"):
			_, _ = w.Write(hardwareTestInventoryItemsJSON(systemDiskNames))

		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))

	t.Cleanup(srv.Close)
	return srv
}

// hardwareTestDeviceNoIPJSON returns a device JSON payload with primary_ip4 absent
// (null), used to verify the missing-primary-IP error path.
func hardwareTestDeviceNoIPJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      hwTestDeviceID,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": "hw-test-01",
		"name":    "hw-test-01",
		"serial":  "SN001",
		"device_type": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/device-types/1/", "display": "test-type",
			"manufacturer": map[string]any{
				"id": 1, "url": "http://localhost/api/dcim/manufacturers/1/",
				"display": "test-mfr", "name": "test-mfr", "slug": "test-mfr",
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
			"display": "test-role", "name": "test-role", "slug": "test-role", "_depth": 0,
		},
		"site": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/sites/1/",
			"display": "test-site", "name": "test-site", "slug": "test-site",
		},
		"platform": map[string]any{
			"id": 1, "url": "http://localhost/api/dcim/platforms/1/",
			"display": "x86_64", "name": "x86_64", "slug": "x86-64",
		},
		"tenant": map[string]any{
			"id": 1, "url": "http://localhost/api/tenancy/tenants/1/",
			"display": "testing", "name": "testing", "slug": "testing",
		},
		// primary_ip and primary_ip4 are intentionally absent (null)
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

// hardwareTestInterfaceNoMACJSON returns a minimal Interface without any MAC address.
func hardwareTestInterfaceNoMACJSON() []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      hwTestInterfaceID,
		"url":     "http://localhost/api/dcim/interfaces/100/",
		"display": "eth0",
		"device": map[string]any{
			"id": hwTestDeviceID, "url": "http://localhost/api/dcim/devices/1/",
			"display": "hw-test-01",
		},
		"name":                          "eth0",
		"type":                          map[string]any{},
		"link_peers":                    []any{},
		"connected_endpoints_reachable": false,
		"count_ipaddresses":             0,
		"count_fhrp_groups":             0,
		"_occupied":                     false,
		// primary_mac_address absent and mac_addresses empty → no MAC
		"mac_addresses": []any{},
	})
	return b
}

// TestCreateHardwares_MissingPrimaryIP verifies that a staged device without a
// primary IPv4 address produces no Hardware objects and no global error.
// The per-device error is logged but not propagated by CreateHardwares.
func TestCreateHardwares_MissingPrimaryIP(t *testing.T) {
	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{hardwareTestDeviceNoIPJSON()},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if strings.HasSuffix(r.URL.Path, "/api/dcim/devices/") {
			_, _ = w.Write(deviceListJSON)
		} else {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)

	nb := netbox.NewAPIClientFor(srv.URL, "fake-token")
	hardwares, err := tinkerbell.CreateHardwares(nb, context.Background(), common.DeviceFilters{}, nil)
	require.NoError(t, err, "CreateHardwares must not return a global error for per-device failures")
	assert.Empty(t, hardwares, "no Hardware should be produced when primary_ip4 is absent")
}

// TestCreateHardwares_MissingMAC verifies that a staged device whose primary
// interface has no MAC address produces no Hardware objects and no global error.
func TestCreateHardwares_MissingMAC(t *testing.T) {
	deviceListJSON, _ := json.Marshal(map[string]any{
		"count": 1, "next": nil, "previous": nil,
		"results": []json.RawMessage{hardwareTestDeviceJSON()},
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		path := r.URL.Path
		q := r.URL.Query()

		switch {
		case strings.HasSuffix(path, "/api/dcim/devices/"):
			_, _ = w.Write(deviceListJSON)
		case strings.HasSuffix(path, "/api/ipam/ip-addresses/"):
			if q.Get("id") != "" {
				_, _ = w.Write(hardwareTestIPAddressJSON())
			} else {
				_, _ = w.Write(hardwareTestEmptyIPListJSON())
			}
		case strings.Contains(path, "/api/dcim/interfaces/"):
			// Return interface without any MAC address
			_, _ = w.Write(hardwareTestInterfaceNoMACJSON())
		case strings.HasSuffix(path, "/api/ipam/prefixes/"):
			_, _ = w.Write(hardwareTestPrefixListJSON())
		case strings.HasSuffix(path, "/api/dcim/inventory-items/"):
			_, _ = w.Write(hardwareTestInventoryItemsJSON([]string{"/dev/sda"}))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"detail":"not found"}`))
		}
	}))
	t.Cleanup(srv.Close)

	nb := netbox.NewAPIClientFor(srv.URL, "fake-token")
	hardwares, err := tinkerbell.CreateHardwares(nb, context.Background(), common.DeviceFilters{}, nil)
	require.NoError(t, err, "CreateHardwares must not return a global error for per-device failures")
	assert.Empty(t, hardwares, "no Hardware should be produced when the interface has no MAC address")
}

// TestCreateHardwares_SystemDisk verifies that Hardware.spec.disks is populated
// from NetBox inventory items with role slug "system-disk".
func TestCreateHardwares_SystemDisk(t *testing.T) {
	tests := []struct {
		name            string
		systemDiskNames []string
		wantHardwares   int
		wantDiskPaths   []string
	}{
		{
			name:            "single system-disk item",
			systemDiskNames: []string{"/dev/nvme0n1"},
			wantHardwares:   1,
			wantDiskPaths:   []string{"/dev/nvme0n1"},
		},
		{
			name:            "multiple system-disk items",
			systemDiskNames: []string{"/dev/sda", "/dev/sdb"},
			wantHardwares:   1,
			wantDiskPaths:   []string{"/dev/sda", "/dev/sdb"},
		},
		{
			name:            "no system-disk item — hardware skipped",
			systemDiskNames: []string{},
			wantHardwares:   0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newHardwareTestServer(t, tc.systemDiskNames)
			nb := netbox.NewAPIClientFor(srv.URL, "fake-token")

			filters := common.DeviceFilters{
				Sites: []string{},
				Roles: []string{},
			}

			hardwares, err := tinkerbell.CreateHardwares(nb, context.Background(), filters, nil)
			require.NoError(t, err)
			require.Len(t, hardwares, tc.wantHardwares)

			if tc.wantHardwares > 0 {
				diskPaths := make([]string, len(hardwares[0].Spec.Disks))
				for i, d := range hardwares[0].Spec.Disks {
					diskPaths[i] = d.Device
				}
				assert.Equal(t, tc.wantDiskPaths, diskPaths)
			}
		})
	}
}
