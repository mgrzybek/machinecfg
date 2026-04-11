/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package netbox_updater

import (
	"context"
	"fmt"
	"log/slog"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"machinecfg/pkg/cluster"
)

var capiClusterGVK = schema.GroupVersionKind{
	Group:   "cluster.x-k8s.io",
	Version: "v1beta1",
	Kind:    "Cluster",
}

// ClusterReconciler watches CAPI Cluster objects and triggers a NetBox sync
// (FHRP groups, IP addresses, Services) whenever a cluster's state changes.
type ClusterReconciler struct {
	client.Client
	Config   *Config
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reads the CAPI Cluster and calls cluster.SyncStatus for that cluster.
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	netboxClient := r.Config.GetNetBoxClient()
	if netboxClient == nil {
		slog.Warn("NetBox client not yet configured, requeueing", "cluster", req.NamespacedName)
		return ctrl.Result{Requeue: true}, nil
	}

	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(capiClusterGVK)
	if err := r.Get(ctx, req.NamespacedName, capiCluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("cannot get CAPI Cluster %s: %w", req.NamespacedName, err)
	}

	results, err := cluster.SyncStatus(
		r.Client,
		ctx,
		req.Namespace,
		netboxClient,
		[]string{capiCluster.GetName()},
	)
	if err != nil {
		r.Recorder.Eventf(capiCluster, corev1.EventTypeWarning, "NetBoxSyncFailed",
			"NetBox sync failed: %v", err)
		return ctrl.Result{}, fmt.Errorf("cannot sync cluster %s to NetBox: %w", req.Name, err)
	}

	for _, result := range results {
		if result.Error != "" {
			r.Recorder.Eventf(capiCluster, corev1.EventTypeWarning, "NetBoxSyncFailed",
				"NetBox sync error: %s", result.Error)
			slog.Warn("NetBox cluster sync error",
				"cluster", req.NamespacedName,
				"error", result.Error,
			)
		} else if result.Updated {
			r.Recorder.Eventf(capiCluster, corev1.EventTypeNormal, "NetBoxUpdated",
				"NetBox cluster records updated")
			slog.Info("NetBox cluster records updated", "cluster", req.NamespacedName)
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager registers the ClusterReconciler to watch CAPI Cluster objects.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("cluster-reconciler")

	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(capiClusterGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(capiCluster).
		Named("cluster").
		Complete(r)
}
