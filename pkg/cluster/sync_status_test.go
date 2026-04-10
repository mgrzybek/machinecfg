/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster_test

import (
	"context"
	"encoding/json"
	"fmt"
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
	clusters         []map[string]any
	devices          []map[string]any
	prefixes         []map[string]any
	fhrpGroups       []map[string]any
	serviceTemplates []map[string]any
	services         []map[string]any
	ipAddresses      []map[string]any
}

// headnodeDeviceJSON builds a minimal NetBox DeviceWithConfigContext JSON payload
// with role=headnode and the given primary IPv4 address (CIDR notation).
// The structure mirrors deviceJSON from show_test.go which is known to deserialise correctly.
func headnodeDeviceJSON(id int, name string, clusterID int, primaryIP string) map[string]any {
	d := deviceJSON(id, name)
	d["role"] = map[string]any{
		"id": 1, "url": "...", "display": "Headnode", "name": "Headnode", "slug": "headnode", "_depth": 0,
	}
	d["cluster"] = map[string]any{
		"id": clusterID, "url": "http://localhost/api/virtualization/clusters/", "display": "cluster", "name": "cluster",
	}
	d["primary_ip4"] = map[string]any{
		"id":      id,
		"url":     "http://localhost/api/ipam/ip-addresses/" + primaryIP + "/",
		"display": primaryIP,
		"address": primaryIP,
		"family":  map[string]any{"value": 4, "label": "IPv4"},
	}
	return d
}

// prefixJSON builds a minimal valid NetBox Prefix JSON payload.
func prefixJSON(id int, prefix, domains string) map[string]any {
	cf := map[string]any{}
	if domains != "" {
		cf["Domains"] = domains
	}
	return map[string]any{
		"id":            id,
		"url":           "http://localhost/api/ipam/prefixes/",
		"display":       prefix,
		"prefix":        prefix,
		"family":        map[string]any{"value": 4, "label": "IPv4"},
		"children":      0,
		"_depth":        0,
		"custom_fields": cf,
	}
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

// clusterEntry builds a NetBox cluster map with the given name and type slug.
func clusterEntry(id int, name, typeSlug string) map[string]any {
	typeName := strings.ToUpper(typeSlug[:1]) + typeSlug[1:]
	return map[string]any{
		"id":      id,
		"url":     "http://localhost/api/virtualization/clusters/" + name + "/",
		"display": name,
		"name":    name,
		"type": map[string]any{
			"id": 1, "url": "...", "display": typeName, "name": typeName, "slug": typeSlug,
		},
	}
}

// addClusters populates state.clusters with the given (name, typeSlug) pairs.
func addClusters(state *netboxState, entries ...struct{ name, typeSlug string }) {
	for i, e := range entries {
		state.clusters = append(state.clusters, clusterEntry(i+1, e.name, e.typeSlug))
	}
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
// It handles virtualization clusters, FHRP groups, service templates, services and IP addresses.
// Clusters are stored in state.clusters and filtered by ?name= and ?type= query parameters.
func newNetboxClusterServer(t *testing.T, state *netboxState) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// Virtualization clusters list — filters by ?name= and ?type= (slug)
	mux.HandleFunc("/api/virtualization/clusters/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		nameFilter := r.URL.Query()["name"]
		typeFilter := r.URL.Query()["type"]
		filtered := make([]map[string]any, 0)
		for _, c := range state.clusters {
			if len(nameFilter) > 0 {
				matched := false
				for _, n := range nameFilter {
					if c["name"] == n {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if len(typeFilter) > 0 {
				ct, _ := c["type"].(map[string]any)
				slug, _ := ct["slug"].(string)
				matched := false
				for _, tf := range typeFilter {
					if slug == tf {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			filtered = append(filtered, c)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(paginatedResponse(len(filtered), filtered))
	})

	// DCIM devices — filters by ?cluster_id= and ?role= (slug)
	mux.HandleFunc("/api/dcim/devices/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		clusterIDFilter := r.URL.Query().Get("cluster_id")
		roleFilter := r.URL.Query().Get("role")
		filtered := make([]map[string]any, 0)
		for _, d := range state.devices {
			if clusterIDFilter != "" {
				cl, _ := d["cluster"].(map[string]any)
				clID := fmt.Sprintf("%v", cl["id"])
				if clID != clusterIDFilter {
					continue
				}
			}
			if roleFilter != "" {
				role, _ := d["role"].(map[string]any)
				slug, _ := role["slug"].(string)
				if slug != roleFilter {
					continue
				}
			}
			filtered = append(filtered, d)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(paginatedResponse(len(filtered), filtered))
	})

	// IPAM prefixes — GET returns all (mock ignores ?contains=), POST creates a new one.
	mux.HandleFunc("/api/ipam/prefixes/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.Method {
		case http.MethodGet:
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(paginatedResponse(len(state.prefixes), state.prefixes))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID := len(state.prefixes) + 1
			prefix, _ := body["prefix"].(string)
			p := prefixJSON(newID, prefix, "")
			state.prefixes = append(state.prefixes, p)
			w.WriteHeader(http.StatusCreated)
			b, _ := json.Marshal(p)
			_, _ = w.Write(b)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
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

	// IP addresses: handles both list/create (/api/ipam/ip-addresses/) and
	// partial update by ID (/api/ipam/ip-addresses/{id}/).
	mux.HandleFunc("/api/ipam/ip-addresses/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Path after the prefix — if non-empty, this is a resource request (/{id}/).
		suffix := strings.TrimPrefix(r.URL.Path, "/api/ipam/ip-addresses/")
		suffix = strings.TrimSuffix(suffix, "/")

		if suffix != "" {
			// PATCH /api/ipam/ip-addresses/{id}/
			if r.Method != http.MethodPatch {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			for i, ip := range state.ipAddresses {
				idVal, _ := ip["id"].(int)
				if fmt.Sprintf("%d", idVal) == suffix {
					for k, v := range body {
						state.ipAddresses[i][k] = v
					}
					w.WriteHeader(http.StatusOK)
					b, _ := json.Marshal(state.ipAddresses[i])
					_, _ = w.Write(b)
					return
				}
			}
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// List or create
		switch r.Method {
		case http.MethodGet:
			q := r.URL.Query().Get("q")
			filtered := make([]map[string]any, 0)
			for _, ip := range state.ipAddresses {
				addr, _ := ip["address"].(string)
				if q == "" || strings.Contains(addr, q) {
					filtered = append(filtered, ip)
				}
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(paginatedResponse(len(filtered), filtered))
		case http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			newID := len(state.ipAddresses) + 1
			addr, _ := body["address"].(string)
			ip := ipAddressJSON(newID, addr, "active", "Active")
			if dnsName, ok := body["dns_name"].(string); ok && dnsName != "" {
				ip["dns_name"] = dnsName
			}
			state.ipAddresses = append(state.ipAddresses, ip)
			w.WriteHeader(http.StatusCreated)
			b, _ := json.Marshal(ip)
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
// all NetBox resources created from scratch, including the IP address in IPAM.
func TestSyncStatus_HappyPath(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
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
	assert.NotZero(t, r.IPAddressID)

	assert.Len(t, state.fhrpGroups, 1)
	assert.Len(t, state.serviceTemplates, 1)
	assert.Len(t, state.services, 1)
	assert.Len(t, state.ipAddresses, 1)

	assert.Equal(t, testClusterName, state.fhrpGroups[0]["name"])
	assert.Equal(t, "Kubernetes endpoint for "+testClusterName, state.services[0]["description"])
	assert.Contains(t, state.ipAddresses[0]["address"], "192.168.3.8")
}

// TestSyncStatus_Idempotent verifies that running sync-status twice does not
// create duplicate NetBox objects.
func TestSyncStatus_Idempotent(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
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
	assert.Len(t, state.ipAddresses, 1, "IP address must not be duplicated")
}

// TestSyncStatus_CAPIClusterNotFound verifies that a missing CAPI Cluster
// records an error on the result and does not call any NetBox write API.
func TestSyncStatus_CAPIClusterNotFound(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	// K8s client has no CAPI Cluster objects
	k8sClient := fake.NewClientBuilder().Build()

	srv := newNetboxClusterServer(t, state)
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
	state := &netboxState{} // no clusters populated

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
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
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})
	srv := newNetboxClusterServer(t, state)
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

// TestSyncStatus_NonKubernetesClusterSkipped verifies that clusters whose NetBox
// type slug is not "kubernetes" are silently ignored (filtered by the API query).
func TestSyncStatus_NonKubernetesClusterSkipped(t *testing.T) {
	state := &netboxState{}
	// Only add a cluster of type "incus" — should be excluded by the ?type=kubernetes filter
	addClusters(state,
		struct{ name, typeSlug string }{"headnodes", "incus"},
		struct{ name, typeSlug string }{"management", "openstack"},
	)

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, nil)
	require.NoError(t, err)
	assert.Empty(t, results, "non-kubernetes clusters must not be processed")
	assert.Empty(t, state.fhrpGroups, "no FHRP group must be created for non-kubernetes clusters")
}

// TestSyncStatus_DNSNameSetOnCreate verifies that dns_name is populated when
// the CAPI Cluster controlPlaneEndpoint.host is a hostname (not a bare IP).
func TestSyncStatus_DNSNameSetOnCreate(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	// Use a hostname as controlPlaneEndpoint.host
	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})
	capiCluster.SetName(testClusterName)
	capiCluster.SetNamespace(testNamespace)
	capiCluster.Object["spec"] = map[string]interface{}{
		"controlPlaneEndpoint": map[string]interface{}{
			"host": "k8s.example.com",
			"port": int64(6443),
		},
	}

	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	assert.True(t, results[0].Updated)
	require.Len(t, state.ipAddresses, 1)
	assert.Equal(t, "k8s.example.com", state.ipAddresses[0]["dns_name"])
}

// TestSyncStatus_DNSNamePatchedOnExisting verifies that dns_name is set via PATCH
// when the IP already exists in NetBox IPAM without a dns_name.
func TestSyncStatus_DNSNamePatchedOnExisting(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	// Pre-populate the IP without dns_name
	existingIP := ipAddressJSON(1, "192.168.3.8/32", "active", "Active")
	state.ipAddresses = append(state.ipAddresses, existingIP)

	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})
	capiCluster.SetName(testClusterName)
	capiCluster.SetNamespace(testNamespace)
	capiCluster.Object["spec"] = map[string]interface{}{
		"controlPlaneEndpoint": map[string]interface{}{
			"host": "k8s.example.com",
			"port": int64(6443),
		},
	}

	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	assert.True(t, results[0].Updated, "updated must be true when dns_name was patched")
	assert.Len(t, state.ipAddresses, 1, "no new IP must be created")
	assert.Equal(t, "k8s.example.com", state.ipAddresses[0]["dns_name"])
}

// TestSyncStatus_DNSNameFromPrefix verifies that when controlPlaneEndpoint.host
// is a bare IP (not a hostname), dns_name is constructed from the parent
// prefix's Domains custom field as "<cluster-name>.<domain>".
func TestSyncStatus_DNSNameFromPrefix(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})
	state.prefixes = append(state.prefixes, prefixJSON(1, "192.168.3.0/24", "k8s.example.com"))

	// controlPlaneEndpoint.host is a bare IP — isHostname returns false
	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443) // host = "192.168.1.100"
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	require.Len(t, state.ipAddresses, 1)
	assert.Equal(t, testClusterName+".k8s.example.com", state.ipAddresses[0]["dns_name"])
}

// TestSyncStatus_DNSNameRoutingOnlyDomains verifies that when all Domains entries
// start with "~" (systemd-networkd routing-only convention), dns_name is not set.
func TestSyncStatus_DNSNameRoutingOnlyDomains(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})
	state.prefixes = append(state.prefixes, prefixJSON(1, "192.168.3.0/24", "~ring0"))

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	require.Len(t, state.ipAddresses, 1)
	assert.Empty(t, state.ipAddresses[0]["dns_name"], "routing-only domains must not be used as dns_name")
}

// TestSyncStatus_DNSNameNoPrefixDomains verifies that when the parent prefix
// has no Domains custom field, dns_name is left empty without error.
func TestSyncStatus_DNSNameNoPrefixDomains(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})
	state.prefixes = append(state.prefixes, prefixJSON(1, "192.168.3.0/24", "")) // no Domains

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "192.168.3.8")
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Empty(t, results[0].Error)
	require.Len(t, state.ipAddresses, 1)
	assert.Empty(t, state.ipAddresses[0]["dns_name"], "dns_name must be absent when no Domains field")
}

// TestSyncStatus_StandaloneKubernetes_HappyPath verifies that a standalone-kubernetes
// cluster is processed correctly: FHRP group and Service are created from the headnode
// primary IP, and no IP address is created in IPAM (no KamajiControlPlane involved).
func TestSyncStatus_StandaloneKubernetes_HappyPath(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "standalone-kubernetes"})
	state.devices = append(state.devices, headnodeDeviceJSON(1, "headnode-1", 1, "10.0.0.1/24"))

	// No CAPI objects in k8s
	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
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
	assert.Zero(t, r.IPAddressID, "no IP address must be created for standalone clusters")

	assert.Len(t, state.fhrpGroups, 1)
	assert.Len(t, state.serviceTemplates, 1)
	assert.Len(t, state.services, 1)
	assert.Empty(t, state.ipAddresses, "no IPAM IP must be created for standalone clusters")
}

// TestSyncStatus_StandaloneKubernetes_Idempotent verifies that running sync-status
// twice on a standalone-kubernetes cluster does not create duplicate NetBox objects.
func TestSyncStatus_StandaloneKubernetes_Idempotent(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "standalone-kubernetes"})
	state.devices = append(state.devices, headnodeDeviceJSON(1, "headnode-1", 1, "10.0.0.1/24"))

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
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

// TestSyncStatus_StandaloneKubernetes_NoHeadnode verifies that a standalone-kubernetes
// cluster with no headnode device records an error and creates no NetBox objects.
func TestSyncStatus_StandaloneKubernetes_NoHeadnode(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "standalone-kubernetes"})
	// state.devices is empty — no headnode

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.NotEmpty(t, r.Error, "error must be recorded when no headnode is found")
	assert.False(t, r.Updated)
	assert.Zero(t, r.FHRPGroupID)
	assert.Empty(t, state.fhrpGroups, "no FHRP group must be created when headnode is missing")
}

// TestSyncStatus_StandaloneKubernetes_NoHeadnodeIP verifies that a headnode device
// with no primary IP records an error and creates no NetBox objects.
// TestSyncStatus_TailscaleSync verifies that when a KamajiControlPlane is
// Tailscale-exposed, the Tailscale IP is written to NetBox IPAM and the result
// carries the TailscaleAddress field.
func TestSyncStatus_TailscaleSync(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlaneWithTailscale(testClusterName, testNamespace, "my-cluster")
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	secret := makeTailscaleSecret("ts-my-cluster", "my-cluster.tailnet.ts.net", []string{"100.64.0.1"})
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp, ss, secret).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Empty(t, r.Error)
	assert.True(t, r.Updated)
	assert.Equal(t, "my-cluster.tailnet.ts.net", r.TailscaleAddress)

	// The Tailscale IP must have been added to IPAM
	tsIPFound := false
	for _, ip := range state.ipAddresses {
		if addr, _ := ip["address"].(string); strings.Contains(addr, "100.64.0.1") {
			tsIPFound = true
			break
		}
	}
	assert.True(t, tsIPFound, "Tailscale IP 100.64.0.1 must be present in IPAM")
	// A /32 prefix must have been created for the Tailscale IP
	assert.NotEmpty(t, state.prefixes, "a prefix must be created for the Tailscale IP")
}

// TestSyncStatus_TailscaleSync_NoStatefulSet verifies that a missing Tailscale StatefulSet
// is handled gracefully (logged as warning) without failing the whole sync.
func TestSyncStatus_TailscaleSync_NoStatefulSet(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "managed-kubernetes"})

	capiCluster := makeCAPICluster(testClusterName, testNamespace, 6443)
	kcp := makeKamajiControlPlaneWithTailscale(testClusterName, testNamespace, "my-cluster")
	// No StatefulSet or Secret
	k8sClient := fake.NewClientBuilder().WithObjects(capiCluster, kcp).Build()

	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.Empty(t, r.Error)
	assert.Empty(t, r.TailscaleAddress, "TailscaleAddress must be empty when StatefulSet is missing")
}

func TestSyncStatus_StandaloneKubernetes_NoHeadnodeIP(t *testing.T) {
	state := &netboxState{}
	addClusters(state, struct{ name, typeSlug string }{testClusterName, "standalone-kubernetes"})
	// Headnode without primary_ip4
	state.devices = append(state.devices, map[string]any{
		"id":      1,
		"display": "headnode-noip",
		"name":    "headnode-noip",
		"role": map[string]any{
			"id": 1, "url": "...", "display": "Headnode", "name": "Headnode", "slug": "headnode",
		},
		"cluster": map[string]any{"id": 1},
		// no primary_ip4, no primary_ip
	})

	k8sClient := fake.NewClientBuilder().Build()
	srv := newNetboxClusterServer(t, state)
	netboxClient := netbox.NewAPIClientFor(srv.URL, "fake-token")

	results, err := cluster.SyncStatus(k8sClient, context.Background(), testNamespace, netboxClient, []string{testClusterName})
	require.NoError(t, err)
	require.Len(t, results, 1)

	r := results[0]
	assert.NotEmpty(t, r.Error, "error must be recorded when headnode has no primary IP")
	assert.False(t, r.Updated)
	assert.Zero(t, r.FHRPGroupID)
	assert.Empty(t, state.fhrpGroups)
}
