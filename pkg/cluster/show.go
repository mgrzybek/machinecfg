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

// ClusterRow holds the combined NetBox + Kubernetes view of a single cluster.
type ClusterRow struct {
	Name         string   `json:"name"`
	Type         string   `json:"type,omitempty"`
	NetBoxStatus string   `json:"netbox-status,omitempty"`
	CAPIReady    string   `json:"capi-ready,omitempty"`
	DeviceCount  int      `json:"device-count"`
	Devices      []string `json:"devices,omitempty"`
}

// Show lists NetBox virtualization clusters (filtered by clusterNames when non-empty)
// and enriches each row with data from the corresponding CAPI Cluster object in
// Kubernetes. Missing data on either side is represented by empty fields so that
// the caller can render a full unified view regardless.
func Show(
	k8sClient client.Client,
	ctx context.Context,
	namespace string,
	netboxClient *netbox.APIClient,
	clusterNames []string,
) ([]ClusterRow, error) {
	req := netboxClient.VirtualizationAPI.VirtualizationClustersList(ctx)
	if len(clusterNames) > 0 && clusterNames[0] != "" {
		req = req.Name(clusterNames)
	}

	netboxClusters, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list NetBox clusters: %w", err)
	}

	if netboxClusters.Count == 0 {
		slog.Warn("no NetBox clusters found", "func", "Show")
		return nil, nil
	}

	var rows []ClusterRow

	for _, nbCluster := range netboxClusters.Results {
		row := ClusterRow{
			Name: nbCluster.Name,
			Type: nbCluster.Type.GetSlug(),
		}

		if nbCluster.Status != nil {
			row.NetBoxStatus = string(nbCluster.Status.GetValue())
		}

		devices, err := getClusterDevices(ctx, netboxClient, nbCluster.Id)
		if err != nil {
			slog.Warn("cannot list devices for cluster", "func", "Show", "cluster", nbCluster.Name, "error", err.Error())
		} else {
			for _, d := range devices {
				row.Devices = append(row.Devices, d.GetName())
			}
		}

		vms, err := getClusterVMs(ctx, netboxClient, nbCluster.Id)
		if err != nil {
			slog.Warn("cannot list VMs for cluster", "func", "Show", "cluster", nbCluster.Name, "error", err.Error())
		} else {
			for _, vm := range vms {
				row.Devices = append(row.Devices, vm.GetName())
			}
		}

		row.DeviceCount = len(row.Devices)

		if nbCluster.Type.GetSlug() == managedKubernetesClusterTypeSlug {
			row.CAPIReady = getCAPIClusterInfo(k8sClient, ctx, namespace, nbCluster.Name)
		}

		rows = append(rows, row)
	}

	return rows, nil
}

// getClusterDevices returns all DCIM devices assigned to the given NetBox cluster ID.
func getClusterDevices(ctx context.Context, netboxClient *netbox.APIClient, clusterID int32) ([]netbox.DeviceWithConfigContext, error) {
	clusterIDPtr := &clusterID
	result, _, err := netboxClient.DcimAPI.DcimDevicesList(ctx).
		ClusterId([]*int32{clusterIDPtr}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list devices for cluster %d: %w", clusterID, err)
	}
	return result.Results, nil
}

// getClusterVMs returns all virtual machines assigned to the given NetBox cluster ID.
func getClusterVMs(ctx context.Context, netboxClient *netbox.APIClient, clusterID int32) ([]netbox.VirtualMachineWithConfigContext, error) {
	clusterIDPtr := &clusterID
	result, _, err := netboxClient.VirtualizationAPI.VirtualizationVirtualMachinesList(ctx).
		ClusterId([]*int32{clusterIDPtr}).
		Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list VMs for cluster %d: %w", clusterID, err)
	}
	return result.Results, nil
}

// getCAPIClusterInfo looks up the CAPI Cluster in Kubernetes and returns the
// readiness status ("true", "false", "unknown", or "" if not found).
func getCAPIClusterInfo(k8sClient client.Client, ctx context.Context, namespace, name string) (ready string) {
	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})

	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, capiCluster); err != nil {
		slog.Debug("CAPI Cluster not found", "func", "getCAPIClusterInfo", "cluster", name, "error", err.Error())
		return ""
	}

	conditions, found, _ := unstructured.NestedSlice(capiCluster.Object, "status", "conditions")
	if !found {
		return ""
	}

	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == "Ready" {
			if s, ok := cond["status"].(string); ok {
				switch s {
				case "True":
					return "true"
				case "False":
					return "false"
				default:
					return "unknown"
				}
			}
		}
	}

	return ""
}
