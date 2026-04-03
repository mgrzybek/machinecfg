/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package tinkerbell

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/netbox-community/go-netbox/v4"
	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const annotationProvisioned = "v1alpha1.tinkerbell.org/provisioned"

// SyncStatusResult holds the outcome of a single device status transition.
type SyncStatusResult struct {
	Hostname string `json:"hostname"`
	DeviceID int32  `json:"device-id"`
	Updated  bool   `json:"updated"`
	Error    string `json:"error,omitempty"`
}

// SyncStatus lists all Hardware objects in the given namespace and reconciles each
// NetBox device status according to the provisioned annotation:
//   - annotation == "true"  → transition NetBox device to "active"
//   - annotation absent     → transition NetBox device to "staged"
//
// Errors on individual devices are recorded in the result slice; the function
// continues processing remaining devices and only returns a fatal error when the
// initial Kubernetes list call fails.
func SyncStatus(k8sClient client.Client, ctx context.Context, namespace string, netboxClient *netbox.APIClient) ([]SyncStatusResult, error) {
	hwList := &tinkerbellKubeObjects.HardwareList{}
	if err := k8sClient.List(ctx, hwList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("cannot list Hardware in namespace %s: %w", namespace, err)
	}

	if len(hwList.Items) == 0 {
		slog.Warn("no Hardware objects found", "func", "SyncStatus", "namespace", namespace)
		return nil, nil
	}

	var results []SyncStatusResult

	for _, hw := range hwList.Items {
		annotationVal, annotationExists := hw.Annotations[annotationProvisioned]

		// Only handle the two explicit cases; ignore any other annotation value.
		isProvisioned := annotationExists && annotationVal == "true"
		isUnprovisioned := !annotationExists

		if !isProvisioned && !isUnprovisioned {
			continue
		}

		result := SyncStatusResult{Hostname: hw.Name}

		deviceIDStr, ok := hw.Labels["netbox-device-id"]
		if !ok || deviceIDStr == "" {
			result.Error = "label netbox-device-id missing"
			slog.Warn("netbox-device-id label missing", "func", "SyncStatus", "name", hw.Name)
			results = append(results, result)
			continue
		}

		deviceID64, err := strconv.ParseInt(deviceIDStr, 10, 32)
		if err != nil {
			result.Error = fmt.Sprintf("invalid netbox-device-id: %s", deviceIDStr)
			slog.Warn("invalid netbox-device-id", "func", "SyncStatus", "name", hw.Name, "value", deviceIDStr)
			results = append(results, result)
			continue
		}

		result.DeviceID = int32(deviceID64)

		var (
			updated    bool
			targetName string
		)

		if isProvisioned {
			updated, err = setDeviceStatus(ctx, netboxClient, result.DeviceID, netbox.DEVICESTATUSVALUE_ACTIVE)
			targetName = "active"
		} else {
			updated, err = setDeviceStatus(ctx, netboxClient, result.DeviceID, netbox.DEVICESTATUSVALUE_STAGED)
			targetName = "staged"
		}

		if err != nil {
			result.Error = err.Error()
			slog.Error("failed to update NetBox device status", "func", "SyncStatus", "name", hw.Name, "device_id", result.DeviceID, "error", err.Error())
		} else {
			result.Updated = updated
			if updated {
				slog.Info("NetBox device status updated", "func", "SyncStatus", "name", hw.Name, "device_id", result.DeviceID, "status", targetName)
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// setDeviceStatus transitions a NetBox device to targetStatus only if its current
// status is not already targetStatus. Returns (true, nil) if the transition was
// applied, (false, nil) if already at target (no change needed), or (false, err)
// on API failure.
func setDeviceStatus(ctx context.Context, netboxClient *netbox.APIClient, deviceID int32, targetStatus netbox.DeviceStatusValue) (bool, error) {
	device, _, err := netboxClient.DcimAPI.DcimDevicesRetrieve(ctx, deviceID).Execute()
	if err != nil {
		return false, fmt.Errorf("cannot retrieve NetBox device %d: %w", deviceID, err)
	}

	current := device.GetStatus()
	if current.GetValue() == targetStatus {
		return false, nil
	}

	patch := netbox.NewPatchedWritableDeviceWithConfigContextRequest()
	patch.SetStatus(targetStatus)

	_, _, err = netboxClient.DcimAPI.DcimDevicesPartialUpdate(ctx, deviceID).
		PatchedWritableDeviceWithConfigContextRequest(*patch).
		Execute()
	if err != nil {
		return false, err
	}

	return true, nil
}
