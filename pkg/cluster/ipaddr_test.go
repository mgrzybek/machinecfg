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
	"strings"
	"testing"

	"github.com/netbox-community/go-netbox/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"machinecfg/pkg/cluster"
)

// makeKamajiControlPlane builds a minimal KamajiControlPlane unstructured object.
func makeKamajiControlPlane(name, namespace string, lbIPs string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1alpha1",
		Kind:    "KamajiControlPlane",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)

	annotations := map[string]interface{}{}
	if lbIPs != "" {
		annotations["io.cilium/lb-ipam-ips"] = lbIPs
	}

	obj.Object["spec"] = map[string]interface{}{
		"network": map[string]interface{}{
			"serviceAnnotations": annotations,
		},
	}
	return obj
}

// ipAddressJSON builds a minimal valid IPAddress JSON payload for the NetBox mock.
func ipAddressJSON(id int, address, statusValue, statusLabel string) map[string]any {
	return map[string]any{
		"id":          id,
		"url":         "http://localhost/api/ipam/ip-addresses/",
		"display":     address,
		"address":     address,
		"family":      map[string]any{"value": 4, "label": "IPv4"},
		"nat_outside": []any{},
		"status": map[string]any{
			"value": statusValue,
			"label": statusLabel,
		},
	}
}

// newIPAddrNetboxServer returns a mock NetBox server for ipaddr show tests.
// addresses maps IP strings to their status (e.g. "192.168.3.8" → "active").
func newIPAddrNetboxServer(t *testing.T, addresses map[string]string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/ipam/ip-addresses/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		q := r.URL.Query().Get("q")

		var results []map[string]any
		for ip, status := range addresses {
			if q == "" || strings.Contains(ip, q) {
				results = append(results, ipAddressJSON(len(results)+1, ip+"/24", status, strings.Title(status)))
			}
		}

		b, _ := json.Marshal(map[string]any{
			"count":    len(results),
			"next":     nil,
			"previous": nil,
			"results":  results,
		})
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestShowIPAddresses_HappyPath verifies the full flow: KamajiControlPlane found,
// IP present in NetBox with status "active".
func TestShowIPAddresses_HappyPath(t *testing.T) {
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(kcp).Build()

	srv := newIPAddrNetboxServer(t, map[string]string{"192.168.3.8": "active"})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, rows, 1)

	r := rows[0]
	assert.Equal(t, testClusterName, r.ClusterName)
	assert.Equal(t, "192.168.3.8", r.IPAddress)
	assert.True(t, r.NetBoxAssigned)
	assert.Equal(t, "active", r.NetBoxStatus)
}

// TestShowIPAddresses_IPNotInNetBox verifies that an IP unknown to NetBox IPAM
// shows NetBoxAssigned=false without causing an error.
func TestShowIPAddresses_IPNotInNetBox(t *testing.T) {
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(kcp).Build()

	srv := newIPAddrNetboxServer(t, map[string]string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, nil)
	require.NoError(t, err)
	require.Len(t, rows, 1)

	assert.Equal(t, "192.168.3.8", rows[0].IPAddress)
	assert.False(t, rows[0].NetBoxAssigned)
	assert.Empty(t, rows[0].NetBoxStatus)
}

// TestShowIPAddresses_MultipleIPs verifies that a comma-separated list of IPs
// produces one row per IP.
func TestShowIPAddresses_MultipleIPs(t *testing.T) {
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8, 192.168.3.9")
	k8sClient := fake.NewClientBuilder().WithObjects(kcp).Build()

	srv := newIPAddrNetboxServer(t, map[string]string{
		"192.168.3.8": "active",
		"192.168.3.9": "reserved",
	})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, nil)
	require.NoError(t, err)
	assert.Len(t, rows, 2)
}

// TestShowIPAddresses_ClusterFilter verifies that --clusters filters by name.
func TestShowIPAddresses_ClusterFilter(t *testing.T) {
	kcp1 := makeKamajiControlPlane("cluster-a", testNamespace, "10.0.0.1")
	kcp2 := makeKamajiControlPlane("cluster-b", testNamespace, "10.0.0.2")
	k8sClient := fake.NewClientBuilder().WithObjects(kcp1, kcp2).Build()

	srv := newIPAddrNetboxServer(t, map[string]string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, []string{"cluster-a"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "cluster-a", rows[0].ClusterName)
}

// TestShowIPAddresses_NoAnnotation verifies that a KamajiControlPlane without
// the Cilium annotation is silently skipped.
func TestShowIPAddresses_NoAnnotation(t *testing.T) {
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "")
	k8sClient := fake.NewClientBuilder().WithObjects(kcp).Build()

	srv := newIPAddrNetboxServer(t, map[string]string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestShowIPAddresses_NoKamajiObjects verifies that an empty namespace returns nil.
func TestShowIPAddresses_NoKamajiObjects(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()

	srv := newIPAddrNetboxServer(t, map[string]string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	rows, err := cluster.ShowIPAddresses(k8sClient, context.Background(), testNamespace, netboxClient, nil)
	require.NoError(t, err)
	assert.Empty(t, rows)
}
