/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/netbox-community/go-netbox/v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const serviceTemplateName = "Kubernetes endpoint"
const defaultControlPlanePort = int32(6443)

// SyncStatusResult holds the outcome of a single cluster sync operation.
type SyncStatusResult struct {
	ClusterName string `json:"cluster-name"`
	FHRPGroupID int32  `json:"fhrp-group-id,omitempty"`
	ServiceID   int32  `json:"service-id,omitempty"`
	Updated     bool   `json:"updated"`
	Error       string `json:"error,omitempty"`
}

// SyncStatus reads NetBox virtualization clusters (filtered by clusterNames) and,
// for each one, ensures that the corresponding FHRP group, ServiceTemplate and
// Service records exist in NetBox, taking the control-plane endpoint port from
// the CAPI Cluster object in Kubernetes.
//
// A missing CAPI Cluster is treated as a per-cluster error (recorded in the
// result) rather than a fatal error so the function continues with the
// remaining clusters.
func SyncStatus(
	k8sClient client.Client,
	ctx context.Context,
	namespace string,
	netboxClient *netbox.APIClient,
	clusterNames []string,
) ([]SyncStatusResult, error) {
	req := netboxClient.VirtualizationAPI.VirtualizationClustersList(ctx)
	if len(clusterNames) > 0 && clusterNames[0] != "" {
		req = req.Name(clusterNames)
	}

	netboxClusters, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list NetBox clusters: %w", err)
	}

	if netboxClusters.Count == 0 {
		slog.Warn("no NetBox clusters found", "func", "SyncStatus")
		return nil, nil
	}

	nextID, err := getNextFHRPGroupID(ctx, netboxClient)
	if err != nil {
		return nil, fmt.Errorf("cannot determine next FHRP group ID: %w", err)
	}

	var results []SyncStatusResult

	for _, nbCluster := range netboxClusters.Results {
		result := SyncStatusResult{ClusterName: nbCluster.Name}

		port, err := getCAPIClusterPort(k8sClient, ctx, namespace, nbCluster.Name)
		if err != nil {
			result.Error = err.Error()
			slog.Warn("cannot get CAPI Cluster", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}

		fhrpGroup, created, err := getOrCreateFHRPGroup(ctx, netboxClient, nbCluster.Name, nextID)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create FHRP group", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if created {
			nextID++
			result.Updated = true
		}
		result.FHRPGroupID = fhrpGroup.Id

		_, templateCreated, err := getOrCreateServiceTemplate(ctx, netboxClient, port)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create ServiceTemplate", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if templateCreated {
			result.Updated = true
		}

		svc, svcCreated, err := getOrCreateService(ctx, netboxClient, fhrpGroup.Id, nbCluster.Name, port)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create Service", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if svcCreated {
			result.Updated = true
		}
		result.ServiceID = svc.GetId()

		results = append(results, result)
	}

	return results, nil
}

// getCAPIClusterPort looks up the CAPI Cluster object in the given namespace and
// returns its controlPlaneEndpoint port. Falls back to defaultControlPlanePort
// if the object exists but the port field is absent or zero.
func getCAPIClusterPort(k8sClient client.Client, ctx context.Context, namespace, name string) (int32, error) {
	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})

	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, capiCluster); err != nil {
		return 0, fmt.Errorf("cannot get CAPI Cluster %s/%s: %w", namespace, name, err)
	}

	port, found, err := unstructured.NestedInt64(capiCluster.Object, "spec", "controlPlaneEndpoint", "port")
	if err != nil || !found || port == 0 {
		slog.Debug("controlPlaneEndpoint.port not found, using default", "func", "getCAPIClusterPort", "cluster", name)
		return defaultControlPlanePort, nil
	}

	return int32(port), nil
}

// getNextFHRPGroupID returns max(group_id)+1 across all existing FHRP groups,
// or 1 if none exist.
func getNextFHRPGroupID(ctx context.Context, netboxClient *netbox.APIClient) (int32, error) {
	result, _, err := netboxClient.IpamAPI.IpamFhrpGroupsList(ctx).
		Limit(1).
		Ordering("-group_id").
		Execute()
	if err != nil {
		return 0, fmt.Errorf("cannot list FHRP groups: %w", err)
	}

	if result.Count == 0 {
		return 1, nil
	}

	return result.Results[0].GetGroupId() + 1, nil
}

// getOrCreateFHRPGroup looks up a FHRP group by name. If none is found it
// creates one with protocol=other and the given groupID. Returns the group,
// a boolean indicating whether it was created, and any error.
func getOrCreateFHRPGroup(ctx context.Context, netboxClient *netbox.APIClient, name string, groupID int32) (*netbox.FHRPGroup, bool, error) {
	existing, _, err := netboxClient.IpamAPI.IpamFhrpGroupsList(ctx).
		Name([]string{name}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list FHRP groups by name %q: %w", name, err)
	}

	if existing.Count > 0 {
		slog.Debug("FHRP group already exists", "func", "getOrCreateFHRPGroup", "name", name)
		return &existing.Results[0], false, nil
	}

	req := netbox.NewFHRPGroupRequest(netbox.BRIEFFHRPGROUPPROTOCOL_OTHER, groupID)
	req.SetName(name)

	created, _, err := netboxClient.IpamAPI.IpamFhrpGroupsCreate(ctx).
		FHRPGroupRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create FHRP group %q: %w", name, err)
	}

	slog.Info("FHRP group created", "func", "getOrCreateFHRPGroup", "name", name, "group_id", groupID)
	return created, true, nil
}

// getOrCreateServiceTemplate looks up the "Kubernetes endpoint" service template.
// If not found it creates it with TCP protocol and the given port.
func getOrCreateServiceTemplate(ctx context.Context, netboxClient *netbox.APIClient, port int32) (*netbox.ServiceTemplate, bool, error) {
	existing, _, err := netboxClient.IpamAPI.IpamServiceTemplatesList(ctx).
		Name([]string{serviceTemplateName}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list ServiceTemplates: %w", err)
	}

	if existing.Count > 0 {
		slog.Debug("ServiceTemplate already exists", "func", "getOrCreateServiceTemplate")
		return &existing.Results[0], false, nil
	}

	req := netbox.NewWritableServiceTemplateRequest(
		serviceTemplateName,
		netbox.PATCHEDWRITABLESERVICEREQUESTPROTOCOL_TCP,
		[]int32{port},
	)

	created, _, err := netboxClient.IpamAPI.IpamServiceTemplatesCreate(ctx).
		WritableServiceTemplateRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create ServiceTemplate: %w", err)
	}

	slog.Info("ServiceTemplate created", "func", "getOrCreateServiceTemplate", "port", port)
	return created, true, nil
}

// getOrCreateService looks up a Service by its expected description. If not
// found it creates one attached to the given FHRP group.
func getOrCreateService(
	ctx context.Context,
	netboxClient *netbox.APIClient,
	fhrpGroupID int32,
	clusterName string,
	port int32,
) (*netbox.Service, bool, error) {
	description := fmt.Sprintf("Kubernetes endpoint for %s", clusterName)

	existing, _, err := netboxClient.IpamAPI.IpamServicesList(ctx).
		Description([]string{description}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list Services: %w", err)
	}

	if existing.Count > 0 {
		slog.Debug("Service already exists", "func", "getOrCreateService", "cluster", clusterName)
		svc := existing.Results[0]
		return &svc, false, nil
	}

	req := netbox.NewWritableServiceRequest(
		"ipam.fhrpgroup",
		int64(fhrpGroupID),
		serviceTemplateName,
		netbox.PATCHEDWRITABLESERVICEREQUESTPROTOCOL_TCP,
		[]int32{port},
	)
	req.SetDescription(description)

	created, _, err := netboxClient.IpamAPI.IpamServicesCreate(ctx).
		WritableServiceRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create Service for cluster %q: %w", clusterName, err)
	}

	slog.Info("Service created", "func", "getOrCreateService", "cluster", clusterName, "fhrp_group_id", fhrpGroupID)
	return created, true, nil
}
