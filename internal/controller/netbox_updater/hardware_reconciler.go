/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package netbox_updater

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/netbox-community/go-netbox/v4"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"machinecfg/pkg/tinkerbell"
)

const (
	annotationProvisioned = "v1alpha1.tinkerbell.org/provisioned"
	labelNetBoxDeviceID   = "netbox-device-id"
)

var hardwareGVK = schema.GroupVersionKind{
	Group:   "tinkerbell.org",
	Version: "v1alpha1",
	Kind:    "Hardware",
}

// HardwareReconciler watches Tinkerbell Hardware objects and syncs their status
// back to NetBox:
//   - annotation provisioned=true → device active
//   - annotation absent           → device staged
type HardwareReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Config   *Config
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=tinkerbell.org,resources=hardware,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile reads the Hardware object and updates the corresponding NetBox device status.
func (r *HardwareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	netboxClient := r.Config.GetNetBoxClient()
	if netboxClient == nil {
		slog.Warn("NetBox client not yet configured, requeueing", "hardware", req.NamespacedName)
		return ctrl.Result{Requeue: true}, nil
	}

	hw := &unstructured.Unstructured{}
	hw.SetGroupVersionKind(hardwareGVK)
	if err := r.Get(ctx, req.NamespacedName, hw); err != nil {
		if errors.IsNotFound(err) {
			// Object deleted after the event was queued — nothing to do.
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("cannot get Hardware %s: %w", req.NamespacedName, err)
	}

	deviceID, err := r.deviceIDFromLabels(hw)
	if err != nil {
		// Missing or invalid label — log and do not requeue; the label will never
		// be set by this controller.
		slog.Warn("skipping Hardware: "+err.Error(), "hardware", req.NamespacedName)
		return ctrl.Result{}, nil
	}

	targetStatus, statusName := r.targetNetBoxStatus(hw)

	updated, err := tinkerbell.SetDeviceStatus(ctx, netboxClient, deviceID, targetStatus)
	if err != nil {
		r.Recorder.Eventf(hw, corev1.EventTypeWarning, "NetBoxUpdateFailed",
			"Failed to set NetBox device %d to %s: %v", deviceID, statusName, err)
		return ctrl.Result{}, fmt.Errorf("cannot set NetBox device %d to %s: %w", deviceID, statusName, err)
	}

	if updated {
		r.Recorder.Eventf(hw, corev1.EventTypeNormal, "NetBoxUpdated",
			"NetBox device %d status set to %s", deviceID, statusName)
		slog.Info("NetBox device status updated",
			"hardware", req.NamespacedName,
			"device_id", deviceID,
			"status", statusName,
		)
	}

	return ctrl.Result{}, nil
}

// deviceIDFromLabels extracts and validates the netbox-device-id label.
func (r *HardwareReconciler) deviceIDFromLabels(hw *unstructured.Unstructured) (int32, error) {
	val, ok := hw.GetLabels()[labelNetBoxDeviceID]
	if !ok || val == "" {
		return 0, fmt.Errorf("label %q missing", labelNetBoxDeviceID)
	}
	id, err := strconv.ParseInt(val, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %q label value %q", labelNetBoxDeviceID, val)
	}
	return int32(id), nil
}

// targetNetBoxStatus returns the NetBox status and its display name based on
// whether the provisioned annotation is set to "true".
func (r *HardwareReconciler) targetNetBoxStatus(hw *unstructured.Unstructured) (netbox.DeviceStatusValue, string) {
	if hw.GetAnnotations()[annotationProvisioned] == "true" {
		return netbox.DEVICESTATUSVALUE_ACTIVE, "active"
	}
	return netbox.DEVICESTATUSVALUE_STAGED, "staged"
}

// SetupWithManager registers the HardwareReconciler to watch Tinkerbell Hardware objects.
func (r *HardwareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Recorder = mgr.GetEventRecorderFor("hardware-reconciler")

	hw := &unstructured.Unstructured{}
	hw.SetGroupVersionKind(hardwareGVK)

	return ctrl.NewControllerManagedBy(mgr).
		For(hw).
		Named("hardware").
		Complete(r)
}
