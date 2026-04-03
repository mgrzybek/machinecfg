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
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"machinecfg/pkg/tinkerbell"
)

// ptr returns a pointer to the given string (helper for optional annotation values).
func ptr(s string) *string { return &s }

// buildScheme returns a runtime.Scheme with Tinkerbell types registered.
func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	require.NoError(t, tinkerbellKubeObjects.AddToScheme(s))
	return s
}

// makeHardware builds a minimal Hardware object for testing.
// Pass nil for provisionedAnnotation to omit the annotation entirely.
func makeHardware(name, namespace, deviceID string, provisionedAnnotation *string) *tinkerbellKubeObjects.Hardware {
	hw := &tinkerbellKubeObjects.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"netbox-device-id": deviceID,
			},
		},
	}
	if provisionedAnnotation != nil {
		hw.Annotations = map[string]string{
			"v1alpha1.tinkerbell.org/provisioned": *provisionedAnnotation,
		}
	}
	return hw
}

// deviceStatusLabel converts a NetBox status value (e.g. "staged") to its
// Title-Case label form required by DeviceStatusLabel validation (e.g. "Staged").
func deviceStatusLabel(value string) string {
	labels := map[string]string{
		"offline":         "Offline",
		"active":          "Active",
		"planned":         "Planned",
		"staged":          "Staged",
		"failed":          "Failed",
		"inventory":       "Inventory",
		"decommissioning": "Decommissioning",
	}
	if l, ok := labels[value]; ok {
		return l
	}
	return value
}

// netboxDeviceJSON produces a minimal but valid JSON payload for a NetBox device.
// All fields required by DeviceWithConfigContext.UnmarshalJSON must be present.
func netboxDeviceJSON(id int, statusValue string) []byte {
	b, _ := json.Marshal(map[string]any{
		"id":      id,
		"url":     "http://localhost/api/dcim/devices/1/",
		"display": "test-device",
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
		"status":                    map[string]string{"value": statusValue, "label": deviceStatusLabel(statusValue)},
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

// newNetboxServer starts an httptest.Server that simulates the NetBox DCIM device API.
// It tracks the current device status and records whether a PATCH was issued.
func newNetboxServer(t *testing.T, deviceID int, initialStatus string) (srv *httptest.Server, currentStatus *string, patchCalled *bool) {
	t.Helper()

	currentStatus = &initialStatus
	patchCalled = new(bool)

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(netboxDeviceJSON(deviceID, *currentStatus))
		case http.MethodPatch:
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if s, ok := body["status"]; ok {
				*currentStatus = s
			}
			*patchCalled = true
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(netboxDeviceJSON(deviceID, *currentStatus))
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))

	t.Cleanup(srv.Close)
	return srv, currentStatus, patchCalled
}

// TestSyncStatus covers all cases of the annotation-based NetBox status reconciliation.
// It specifically guards against the regression where a device in a status other than
// "staged" (e.g. "planned") was not transitioned to "active" even when the Hardware
// carried the provisioned=true annotation.
func TestSyncStatus(t *testing.T) {
	const (
		namespace = "mushroomcloud"
		hwName    = "cn-0"
		deviceID  = 1
	)

	tests := []struct {
		name                string
		provisionedAnnot    *string // nil = annotation absent
		initialNetboxStatus string
		wantUpdated         bool
		wantFinalStatus     string
		wantPatchCalled     bool
		wantError           string // non-empty means result.Error must contain this
	}{
		{
			// THE REGRESSION CASE: device was "planned", not "staged".
			// Before the fix, setDeviceStatus checked expectedCurrent==staged and returned
			// updated=false. After the fix it only skips when already at target.
			name:                "provisioned=true, device planned → transition to active",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: "planned",
			wantUpdated:         true,
			wantFinalStatus:     "active",
			wantPatchCalled:     true,
		},
		{
			name:                "provisioned=true, device staged → transition to active",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: "staged",
			wantUpdated:         true,
			wantFinalStatus:     "active",
			wantPatchCalled:     true,
		},
		{
			name:                "provisioned=true, device already active → no change",
			provisionedAnnot:    ptr("true"),
			initialNetboxStatus: "active",
			wantUpdated:         false,
			wantFinalStatus:     "active",
			wantPatchCalled:     false,
		},
		{
			name:                "annotation absent, device active → transition to staged",
			provisionedAnnot:    nil,
			initialNetboxStatus: "active",
			wantUpdated:         true,
			wantFinalStatus:     "staged",
			wantPatchCalled:     true,
		},
		{
			name:                "annotation absent, device already staged → no change",
			provisionedAnnot:    nil,
			initialNetboxStatus: "staged",
			wantUpdated:         false,
			wantFinalStatus:     "staged",
			wantPatchCalled:     false,
		},
		{
			name:                "provisioned=false → skipped entirely",
			provisionedAnnot:    ptr("false"),
			initialNetboxStatus: "planned",
			// isProvisioned=false, isUnprovisioned=false → continue, no result entry
			wantUpdated:     false,
			wantFinalStatus: "planned",
			wantPatchCalled: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, finalStatus, patchCalled := newNetboxServer(t, deviceID, tc.initialNetboxStatus)

			hw := makeHardware(hwName, namespace, "1", tc.provisionedAnnot)
			k8sClient := fake.NewClientBuilder().
				WithScheme(buildScheme(t)).
				WithObjects(hw).
				Build()

			netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

			results, err := tinkerbell.SyncStatus(k8sClient, context.Background(), namespace, netboxClient)
			require.NoError(t, err)

			if tc.provisionedAnnot != nil && *tc.provisionedAnnot == "false" {
				// annotation value "false" is neither provisioned nor unprovisioned → no result
				assert.Empty(t, results)
			} else {
				require.Len(t, results, 1)
				result := results[0]
				assert.Equal(t, hwName, result.Hostname)
				assert.Equal(t, int32(deviceID), result.DeviceID)
				assert.Equal(t, tc.wantUpdated, result.Updated)
				if tc.wantError != "" {
					assert.Contains(t, result.Error, tc.wantError)
				} else {
					assert.Empty(t, result.Error)
				}
			}

			assert.Equal(t, tc.wantPatchCalled, *patchCalled, "unexpected PATCH call state")
			assert.Equal(t, tc.wantFinalStatus, *finalStatus, "unexpected final NetBox status")
		})
	}
}

// TestSyncStatus_MissingDeviceIDLabel verifies that a Hardware without the
// netbox-device-id label produces an error entry and no NetBox PATCH call.
func TestSyncStatus_MissingDeviceIDLabel(t *testing.T) {
	const namespace = "mushroomcloud"

	srv, _, patchCalled := newNetboxServer(t, 1, "staged")

	hw := &tinkerbellKubeObjects.Hardware{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "broken-node",
			Namespace: namespace,
			// No labels at all
			Annotations: map[string]string{
				"v1alpha1.tinkerbell.org/provisioned": "true",
			},
		},
	}

	k8sClient := fake.NewClientBuilder().
		WithScheme(buildScheme(t)).
		WithObjects(hw).
		Build()

	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := tinkerbell.SyncStatus(k8sClient, context.Background(), namespace, netboxClient)
	require.NoError(t, err)

	require.Len(t, results, 1)
	assert.Equal(t, "broken-node", results[0].Hostname)
	assert.NotEmpty(t, results[0].Error)
	assert.Contains(t, results[0].Error, "netbox-device-id")
	assert.False(t, results[0].Updated)
	assert.False(t, *patchCalled)
}

// TestSyncStatus_NoHardwareObjects verifies that an empty namespace returns no results.
func TestSyncStatus_NoHardwareObjects(t *testing.T) {
	const namespace = "empty-ns"

	srv, _, patchCalled := newNetboxServer(t, 1, "staged")

	k8sClient := fake.NewClientBuilder().
		WithScheme(buildScheme(t)).
		Build()

	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := tinkerbell.SyncStatus(k8sClient, context.Background(), namespace, netboxClient)
	require.NoError(t, err)
	assert.Empty(t, results)
	assert.False(t, *patchCalled)
}
