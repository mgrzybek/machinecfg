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

// SyncStatus lists all Hardware objects in the given namespace and, for each one
// whose annotation v1alpha1.tinkerbell.org/provisioned is "true", transitions the
// corresponding NetBox device from staged to active.
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
		if hw.Annotations[annotationProvisioned] != "true" {
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

		updated, err := setDeviceActive(ctx, netboxClient, result.DeviceID)
		if err != nil {
			result.Error = err.Error()
			slog.Error("failed to update NetBox device status", "func", "SyncStatus", "name", hw.Name, "device_id", result.DeviceID, "error", err.Error())
		} else {
			result.Updated = updated
			if updated {
				slog.Info("NetBox device transitioned to active", "func", "SyncStatus", "name", hw.Name, "device_id", result.DeviceID)
			}
		}

		results = append(results, result)
	}

	return results, nil
}

// setDeviceActive transitions a NetBox device to "active" only if its current status
// is "staged". Returns (true, nil) if the transition was applied, (false, nil) if no
// change was needed, or (false, err) on API failure.
func setDeviceActive(ctx context.Context, netboxClient *netbox.APIClient, deviceID int32) (bool, error) {
	device, _, err := netboxClient.DcimAPI.DcimDevicesRetrieve(ctx, deviceID).Execute()
	if err != nil {
		return false, fmt.Errorf("cannot retrieve NetBox device %d: %w", deviceID, err)
	}

	status := device.GetStatus()
	if status.GetValue() != netbox.DEVICESTATUSVALUE_STAGED {
		return false, nil
	}

	patch := netbox.NewPatchedWritableDeviceWithConfigContextRequest()
	patch.SetStatus(netbox.DEVICESTATUSVALUE_ACTIVE)

	_, _, err = netboxClient.DcimAPI.DcimDevicesPartialUpdate(ctx, deviceID).
		PatchedWritableDeviceWithConfigContextRequest(*patch).
		Execute()
	if err != nil {
		return false, err
	}

	return true, nil
}
