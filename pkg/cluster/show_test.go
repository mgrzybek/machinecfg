/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"machinecfg/pkg/cluster"
)

// makeCAPIClusterWithStatus builds a CAPI Cluster object with a Ready condition.
func makeCAPIClusterWithStatus(name, namespace, host, readyStatus string) *unstructured.Unstructured {
	obj := makeCAPICluster(name, namespace, 6443)
	obj.Object["spec"] = map[string]interface{}{
		"controlPlaneEndpoint": map[string]interface{}{
			"host": host,
			"port": int64(6443),
		},
	}
	obj.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{
				"type":   "Ready",
				"status": readyStatus,
			},
		},
	}
	return obj
}

// deviceJSON builds a minimal valid DeviceWithConfigContext JSON payload.
func deviceJSON(id int, name string) map[string]any {
	return map[string]any{
		"id":      id,
		"url":     "http://localhost/api/dcim/devices/" + name + "/",
		"display": name,
		"name":    name,
		"device_type": map[string]any{
			"id": 1, "url": "...", "display": "dt", "manufacturer": map[string]any{
				"id": 1, "url": "...", "display": "mfr", "name": "mfr", "slug": "mfr",
			},
			"model": "m", "slug": "m",
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
			"id": 1, "url": "...", "display": "r", "name": "r", "slug": "r", "_depth": 0,
		},
		"site": map[string]any{
			"id": 1, "url": "...", "display": "s", "name": "s", "slug": "s",
		},
		"status":                    map[string]any{"value": "active", "label": "Active"},
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

// clusterStatusLabel maps a NetBox cluster status value to its Title-Case label.
func clusterStatusLabel(value string) string {
	labels := map[string]string{
		"planned":         "Planned",
		"staging":         "Staging",
		"active":          "Active",
		"decommissioning": "Decommissioning",
		"offline":         "Offline",
	}
	if l, ok := labels[value]; ok {
		return l
	}
	return value
}

// clusterStatusJSON builds a ClusterStatus JSON object.
func clusterStatusJSON(value string) map[string]any {
	return map[string]any{"value": value, "label": clusterStatusLabel(value)}
}

// newShowServer starts a mock NetBox server for cluster show tests.
func newShowServer(t *testing.T, clusterID int, clusterName, statusValue string, deviceNames []string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	mux.HandleFunc("/api/virtualization/clusters/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		results := []map[string]any{
			{
				"id":      clusterID,
				"url":     "http://localhost/api/virtualization/clusters/",
				"display": clusterName,
				"name":    clusterName,
				"type":    map[string]any{"id": 1, "url": "...", "display": "Managed Kubernetes", "name": "Managed Kubernetes", "slug": "managed-kubernetes"},
				"status":  clusterStatusJSON(statusValue),
			},
		}
		b, _ := json.Marshal(map[string]any{"count": 1, "next": nil, "previous": nil, "results": results})
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		devices := make([]map[string]any, 0, len(deviceNames))
		for i, n := range deviceNames {
			devices = append(devices, deviceJSON(i+1, n))
		}
		b, _ := json.Marshal(map[string]any{"count": len(devices), "next": nil, "previous": nil, "results": devices})
		_, _ = w.Write(b)
	})

	mux.HandleFunc("/api/virtualization/virtual-machines/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		b, _ := json.Marshal(map[string]any{"count": 0, "next": nil, "previous": nil, "results": []any{}})
		_, _ = w.Write(b)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestShow_HappyPath verifies the full show flow with CAPI cluster ready.
func TestShow_HappyPath(t *testing.T) {
	capiCluster := makeCAPIClusterWithStatus(testClusterName, testNamespace, "192.168.1.100", "True")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newShowServer(t, 1, testClusterName, "active", []string{"node1", "node2"})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	r := rows[0]
	assert.Equal(t, testClusterName, r.Name)
	assert.Equal(t, "active", r.NetBoxStatus)
	assert.Equal(t, "true", r.CAPIReady)
	assert.Equal(t, "192.168.1.100", r.ControlPlaneHost)
	assert.Equal(t, 2, r.DeviceCount)
	assert.ElementsMatch(t, []string{"node1", "node2"}, r.Devices)
}

// TestShow_CAPINotFound verifies that a missing CAPI Cluster shows empty K8s fields
// without causing an error (the NetBox data is still returned).
func TestShow_CAPINotFound(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()

	srv := newShowServer(t, 1, testClusterName, "active", []string{"node1"})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	r := rows[0]
	assert.Equal(t, testClusterName, r.Name)
	assert.Equal(t, "active", r.NetBoxStatus)
	assert.Empty(t, r.CAPIReady, "CAPIReady must be empty when CAPI Cluster is absent")
	assert.Empty(t, r.ControlPlaneHost, "ControlPlaneHost must be empty when CAPI Cluster is absent")
	assert.Equal(t, 1, r.DeviceCount)
}

// TestShow_CAPINotReady verifies that a non-ready CAPI Cluster shows "false".
func TestShow_CAPINotReady(t *testing.T) {
	capiCluster := makeCAPIClusterWithStatus(testClusterName, testNamespace, "10.0.0.1", "False")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newShowServer(t, 1, testClusterName, "staging", []string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Equal(t, "false", rows[0].CAPIReady)
}

// TestShow_NoNetboxClusters verifies that an empty NetBox result returns nil.
func TestShow_NoNetboxClusters(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/virtualization/clusters/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		b, _ := json.Marshal(map[string]any{"count": 0, "next": nil, "previous": nil, "results": []any{}})
		_, _ = w.Write(b)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{"nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestShow_NoDevices verifies that a cluster with no devices is shown with DeviceCount=0.
func TestShow_NoDevices(t *testing.T) {
	capiCluster := makeCAPIClusterWithStatus(testClusterName, testNamespace, "10.0.0.1", "True")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newShowServer(t, 1, testClusterName, "active", []string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Equal(t, 0, rows[0].DeviceCount)
	assert.Empty(t, rows[0].Devices)
}

// TestShow_TailscaleAddress verifies that a managed-kubernetes cluster with a
// Tailscale-exposed KamajiControlPlane shows the Tailscale address.
func TestShow_TailscaleAddress(t *testing.T) {
	kcp := makeKamajiControlPlaneWithTailscale(testClusterName, testNamespace, "my-cluster")
	capiCluster := makeCAPIClusterWithStatus(testClusterName, testNamespace, "10.0.0.1", "True")
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	secret := makeTailscaleSecret("ts-my-cluster", "my-cluster.tailnet.ts.net", nil)
	k8sClient := fake.NewClientBuilder().WithObjects(kcp, capiCluster, ss, secret).Build()

	srv := newShowServer(t, 1, testClusterName, "active", []string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Equal(t, "my-cluster.tailnet.ts.net", rows[0].TailscaleAddress)
}

// TestShow_NoTailscaleExposure verifies that a cluster without Tailscale annotations
// has an empty TailscaleAddress field.
func TestShow_NoTailscaleExposure(t *testing.T) {
	capiCluster := makeCAPIClusterWithStatus(testClusterName, testNamespace, "10.0.0.1", "True")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newShowServer(t, 1, testClusterName, "active", []string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.Show(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Empty(t, rows[0].TailscaleAddress)
}
