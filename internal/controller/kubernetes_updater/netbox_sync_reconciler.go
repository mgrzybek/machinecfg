/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package kubernetes_updater

import (
	"context"
	"fmt"
	"log/slog"

	tinkerbellv1 "github.com/tinkerbell/tink/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"machinecfg/pkg/tinkerbell"
)

// NetBoxSyncReconciler polls NetBox on a configurable interval and reconciles
// Tinkerbell Hardware objects in Kubernetes accordingly:
//   - NetBox device staged           → create or update Hardware
//   - NetBox device offline/planned  → delete Hardware
//
// The reconciler is triggered by the ConfigMap (immediate reaction to config
// changes) and re-enqueues itself via RequeueAfter for periodic polling.
type NetBoxSyncReconciler struct {
	client.Client
	Config    *Config
	Namespace string
	Recorder  record.EventRecorder
}

// +kubebuilder:rbac:groups=tinkerbell.org,resources=hardware,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is triggered by the ConfigMap and re-enqueues itself at the
// configured sync interval so that NetBox is polled periodically.
func (r *NetBoxSyncReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.Config.GetNetBoxClient() == nil {
		slog.Warn("NetBox client not yet configured, requeueing", "after", r.Config.GetSyncInterval())
		return ctrl.Result{RequeueAfter: r.Config.GetSyncInterval()}, nil
	}

	switch r.Config.GetBackend() {
	case "tinkerbell", "":
		if err := r.syncTinkerbell(ctx); err != nil {
			slog.Error("Tinkerbell sync failed", "error", err.Error())
			return ctrl.Result{RequeueAfter: r.Config.GetSyncInterval()}, err
		}
	case "metal3":
		// Metal3 BaremetalHost support is planned in Epic 8.
		slog.Info("metal3 backend selected — BaremetalHost sync not yet implemented")
	default:
		slog.Warn("unknown backend, skipping sync", "backend", r.Config.GetBackend())
	}

	slog.Info("NetBox sync completed",
		"backend", r.Config.GetBackend(),
		"requeue_after", r.Config.GetSyncInterval(),
	)
	return ctrl.Result{RequeueAfter: r.Config.GetSyncInterval()}, nil
}

// syncTinkerbell reconciles Tinkerbell Hardware objects against the current
// NetBox device inventory.
func (r *NetBoxSyncReconciler) syncTinkerbell(ctx context.Context) error {
	netboxClient := r.Config.GetNetBoxClient()
	filters := r.Config.GetDeviceFilters()
	ignitionVariant := r.Config.GetIgnitionVariant()

	// Desired state: staged devices → Hardware objects to create/update.
	desired, err := tinkerbell.CreateHardwares(netboxClient, ctx, filters, ignitionVariant)
	if err != nil {
		return fmt.Errorf("cannot build desired Hardware list from NetBox: %w", err)
	}

	// Prune list: offline/planned devices → Hardware objects to delete.
	toPrune, err := tinkerbell.CreateHardwaresToPrune(netboxClient, ctx, filters)
	if err != nil {
		return fmt.Errorf("cannot build Hardware prune list from NetBox: %w", err)
	}

	slog.Info("NetBox Hardware sync",
		"desired", len(desired),
		"to_prune", len(toPrune),
	)

	var syncErrors int

	for i := range desired {
		if err := r.applyHardware(ctx, &desired[i]); err != nil {
			slog.Error("cannot apply Hardware",
				"name", desired[i].Name,
				"namespace", desired[i].Namespace,
				"error", err.Error(),
			)
			syncErrors++
		}
	}

	for i := range toPrune {
		if err := r.pruneHardware(ctx, &toPrune[i]); err != nil {
			slog.Error("cannot prune Hardware",
				"name", toPrune[i].Name,
				"namespace", toPrune[i].Namespace,
				"error", err.Error(),
			)
			syncErrors++
		}
	}

	if syncErrors > 0 {
		return fmt.Errorf("%d Hardware object(s) failed to sync", syncErrors)
	}
	return nil
}

// applyHardware creates the Hardware object if it does not exist, or reconciles
// its labels/annotations if it already exists.
func (r *NetBoxSyncReconciler) applyHardware(ctx context.Context, hw *tinkerbellv1.Hardware) error {
	existing := &tinkerbellv1.Hardware{}
	err := r.Get(ctx, types.NamespacedName{Name: hw.Name, Namespace: hw.Namespace}, existing)
	switch {
	case errors.IsNotFound(err):
		if createErr := r.Create(ctx, hw); createErr != nil {
			return fmt.Errorf("cannot create Hardware %s/%s: %w", hw.Namespace, hw.Name, createErr)
		}
		slog.Info("Hardware created", "name", hw.Name, "namespace", hw.Namespace)
		return nil
	case err != nil:
		return fmt.Errorf("cannot get Hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	// Object exists — delegate to the shared reconcile helper which patches
	// labels and handles the provisioned→active transition.
	if reconcileErr := tinkerbell.ReconcileExistingHardware(r.Client, ctx, hw, r.Config.GetNetBoxClient()); reconcileErr != nil {
		return fmt.Errorf("cannot reconcile Hardware %s/%s: %w", hw.Namespace, hw.Name, reconcileErr)
	}
	return nil
}

// pruneHardware deletes the Hardware object from Kubernetes if it still exists.
func (r *NetBoxSyncReconciler) pruneHardware(ctx context.Context, hw *tinkerbellv1.Hardware) error {
	existing := &tinkerbellv1.Hardware{}
	err := r.Get(ctx, types.NamespacedName{Name: hw.Name, Namespace: hw.Namespace}, existing)
	if errors.IsNotFound(err) {
		return nil // Already gone.
	}
	if err != nil {
		return fmt.Errorf("cannot get Hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	if deleteErr := r.Delete(ctx, existing); deleteErr != nil {
		return fmt.Errorf("cannot delete Hardware %s/%s: %w", hw.Namespace, hw.Name, deleteErr)
	}
	slog.Info("Hardware pruned", "name", existing.Name, "namespace", existing.Namespace)

	// Emit the event on the ConfigMap (breadcrumb after Hardware deletion).
	cm := &corev1.ConfigMap{}
	if getErr := r.Get(ctx, types.NamespacedName{Name: ConfigMapName, Namespace: r.Namespace}, cm); getErr == nil {
		r.Recorder.Eventf(cm, corev1.EventTypeNormal, "HardwarePruned",
			"Hardware %s/%s deleted (device is offline/planned in NetBox)",
			hw.Namespace, hw.Name,
		)
	}
	return nil
}

// SetupWithManager registers the NetBoxSyncReconciler to watch the controller ConfigMap.
func (r *NetBoxSyncReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("netbox-sync-reconciler")

	cmPredicate := predicate.NewPredicateFuncs(func(o client.Object) bool {
		return o.GetName() == ConfigMapName && o.GetNamespace() == r.Namespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}, builder.WithPredicates(cmPredicate)).
		Named("netbox-sync").
		Complete(r)
}
