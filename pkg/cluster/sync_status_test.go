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
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"machinecfg/pkg/cluster"
)

const (
	testNamespace   = "capi-system"
	testClusterName = "my-cluster"
)

// makeCAPICluster returns a CAPI Cluster unstructured object with the given port.
func makeCAPICluster(name, namespace string, port int64) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object["spec"] = map[string]interface{}{
		"controlPlaneEndpoint": map[string]interface{}{
			"host": "192.168.1.100",
			"port": port,
		},
	}
	return obj
}

// netboxState tracks the in-memory state of a mock NetBox server.
type netboxState struct {
	fhrpGroups       []map[string]any
	serviceTemplates []map[string]any
	services         []map[string]any
}

func paginatedResponse(count int, results any) []byte {
	b, _ := json.Marshal(map[string]any{
		"count":    count,
		"next":     nil,
		"previous": nil,
		"results":  results,
	})
	return b
}

func clusterListJSON(names ...string) []byte {
	results := make([]map[string]any, 0, len(names))
	for i, n := range names {
		results = append(results, map[string]any{
			"id":      i + 1,
			"url":     "http://localhost/api/virtualization/clusters/" + n + "/",
			"display": n,
			"name":    n,
			"type": map[string]any{
				"id": 1, "url": "...", "display": "Kubernetes", "name": "Kubernetes", "slug": "kubernetes",
			},
		})
	}
	return paginatedResponse(len(results), results)
}

func fhrpGroupJSON(id int, name string, groupID int) map[string]any {
	return map[string]any{
		"id":           id,
		"url":          "http://localhost/api/ipam/fhrp-groups/",
		"display":      name,
		"name":         name,
		"group_id":     groupID,
		"protocol":     "other",
		"ip_addresses": []any{},
	}
}

func serviceTemplateJSON(id int, name string, port int) map[string]any {
	return map[string]any{
		"id":       id,
		"url":      "http://localhost/api/ipam/service-templates/",
		"display":  name,
		"name":     name,
		"protocol": map[string]any{"value": "tcp", "label": "TCP"},
		"ports":    []int{port},
	}
}

func serviceJSON(id int, desc string) map[string]any {
	return map[string]any{
		"id":                 id,
		"url":                "http://localhost/api/ipam/services/",
		"display":            desc,
		"name":               "Kubernetes endpoint",
		"protocol":           map[string]any{"value": "tcp", "label": "TCP"},
		"ports":              []int{6443},
		"description":        desc,
		"parent_object_type": "ipam.fhrpgroup",
		"parent_object_id":   1,
	}
}

// newNetboxClusterServer returns a minimal NetBox mock server for cluster sync-status.
// It handles virtualization clusters, FHRP groups, service templates and services.
func newNetboxClusterServer(t *testing.T, state *netboxState, clusterNames []string) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Virtualization clusters list
	mux.HandleFunc("/api/virtualization/clusters/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(clusterListJSON(clusterNames...))
	})

	// FHRP groups
	mux.HandleFunc("/api/ipam/fhrp-groups/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			nameFilter := r.URL.Query()["name"]
			filtered := make([]map[string]any, 0)
			for _, g := range state.fhrpGroups {
				if len(nameFilter) == 0 {
					filtered = append(filtered, g)
				} else {
					for _, n := range nameFilter {
						if g["name"] == n {
							filtered = append(filtered, g)
						}
					}
				}
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(paginatedResponse(len(filtered), filtered))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID := len(state.fhrpGroups) + 1
			groupID := int(0)
			if gid, ok := body["group_id"].(float64); ok {
				groupID = int(gid)
			}
			name, _ := body["name"].(string)
			g := fhrpGroupJSON(newID, name, groupID)
			state.fhrpGroups = append(state.fhrpGroups, g)
			w.WriteHeader(http.StatusCreated)
			b, _ := json.Marshal(g)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Service templates
	mux.HandleFunc("/api/ipam/service-templates/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			nameFilter := r.URL.Query()["name"]
			filtered := make([]map[string]any, 0)
			for _, st := range state.serviceTemplates {
				if len(nameFilter) == 0 {
					filtered = append(filtered, st)
				} else {
					for _, n := range nameFilter {
						if st["name"] == n {
							filtered = append(filtered, st)
						}
					}
				}
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(paginatedResponse(len(filtered), filtered))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID := len(state.serviceTemplates) + 1
			name, _ := body["name"].(string)
			ports := []int{6443}
			if ps, ok := body["ports"].([]interface{}); ok && len(ps) > 0 {
				if p, ok := ps[0].(float64); ok {
					ports = []int{int(p)}
				}
			}
			st := serviceTemplateJSON(newID, name, ports[0])
			state.serviceTemplates = append(state.serviceTemplates, st)
			w.WriteHeader(http.StatusCreated)
			b, _ := json.Marshal(st)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	// Services
	mux.HandleFunc("/api/ipam/services/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			descFilter := r.URL.Query()["description"]
			filtered := make([]map[string]any, 0)
			for _, svc := range state.services {
				if len(descFilter) == 0 {
					filtered = append(filtered, svc)
				} else {
					for _, d := range descFilter {
						if svc["description"] == d {
							filtered = append(filtered, svc)
						}
					}
				}
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(paginatedResponse(len(filtered), filtered))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID := len(state.services) + 1
			desc, _ := body["description"].(string)
			svc := serviceJSON(newID, desc)
			state.services = append(state.services, svc)
			w.WriteHeader(http.StatusCreated)
			b, _ := json.Marshal(svc)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// TestSyncStatus_HappyPath verifies the full creation flow: CAPI cluster found,
// all NetBox resources created from scratch.
func TestSyncStatus_HappyPath(t *testing.T) {
	state := &netboxState{}

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newNetboxClusterServer(t, state, []string{testClusterName})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, testClusterName, r.ClusterName)
	assert.Empty(t, r.Error)
	assert.True(t, r.Updated)
	assert.NotZero(t, r.FHRPGroupID)
	assert.NotZero(t, r.ServiceID)

	assert.Len(t, state.fhrpGroups, 1)
	assert.Len(t, state.serviceTemplates, 1)
	assert.Len(t, state.services, 1)

	assert.Equal(t, testClusterName, state.fhrpGroups[0]["name"])
	assert.Equal(t, "Kubernetes endpoint for "+testClusterName, state.services[0]["description"])
}

// TestSyncStatus_Idempotent verifies that running sync-status twice does not
// create duplicate NetBox objects.
func TestSyncStatus_Idempotent(t *testing.T) {
	state := &netboxState{}

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()

	srv := newNetboxClusterServer(t, state, []string{testClusterName})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	ctx := context.Background()

	_, err := cluster.SyncStatus(k8sClient, ctx, testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)

	results, err := cluster.SyncStatus(k8sClient, ctx, testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Empty(t, r.Error)
	assert.False(t, r.Updated, "second run must not report updates")

	assert.Len(t, state.fhrpGroups, 1, "FHRP group must not be duplicated")
	assert.Len(t, state.serviceTemplates, 1, "ServiceTemplate must not be duplicated")
	assert.Len(t, state.services, 1, "Service must not be duplicated")
}

// TestSyncStatus_CAPIClusterNotFound verifies that a missing CAPI Cluster
// records an error on the result and does not call any NetBox write API.
func TestSyncStatus_CAPIClusterNotFound(t *testing.T) {
	state := &netboxState{}

	// K8s client has no CAPI Cluster objects
	k8sClient := fake.NewClientBuilder().Build()

	srv := newNetboxClusterServer(t, state, []string{testClusterName})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Equal(t, testClusterName, r.ClusterName)
	assert.NotEmpty(t, r.Error)
	assert.False(t, r.Updated)
	assert.Zero(t, r.FHRPGroupID)

	assert.Empty(t, state.fhrpGroups, "no FHRP group must be created when CAPI Cluster is missing")
}

// TestSyncStatus_NoNetboxClusters verifies that an empty NetBox cluster list
// returns nil results without error.
func TestSyncStatus_NoNetboxClusters(t *testing.T) {
	state := &netboxState{}

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state, []string{})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{"nonexistent"})
	require.NoError(t, err)
	assert.Empty(t, results)
}

// TestSyncStatus_DefaultPort verifies that when controlPlaneEndpoint.port is
// absent, the service is created on port 6443.
func TestSyncStatus_DefaultPort(t *testing.T) {
	state := &netboxState{}

	// CAPI Cluster without controlPlaneEndpoint.port
	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})
	capiCluster.SetName(testClusterName)
	capiCluster.SetNamespace(testNamespace)
	// No spec.controlPlaneEndpoint.port

	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster).Build()
	srv := newNetboxClusterServer(t, state, []string{testClusterName})
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Empty(t, results[0].Error)
	require.Len(t, state.serviceTemplates, 1)
	ports, _ := state.serviceTemplates[0]["ports"].([]int)
	if len(ports) > 0 {
		assert.Equal(t, 6443, ports[0])
	}
}
