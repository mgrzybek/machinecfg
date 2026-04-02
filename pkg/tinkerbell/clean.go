/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package tinkerbell

import (
	"context"
	"fmt"
	"log/slog"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// CleanUserData sets UserData to nil on one or all Hardware objects in a namespace.
// If hostname is empty, all Hardware objects in the namespace are patched.
func CleanUserData(k8sClient client.Client, ctx context.Context, namespace, hostname string) error {
	return cleanField(k8sClient, ctx, namespace, hostname, func(hw *tinkerbellKubeObjects.Hardware) {
		hw.Spec.UserData = nil
	}, "CleanUserData", "userData")
}

// CleanVendorData sets VendorData to nil on one or all Hardware objects in a namespace.
// If hostname is empty, all Hardware objects in the namespace are patched.
func CleanVendorData(k8sClient client.Client, ctx context.Context, namespace, hostname string) error {
	return cleanField(k8sClient, ctx, namespace, hostname, func(hw *tinkerbellKubeObjects.Hardware) {
		hw.Spec.VendorData = nil
	}, "CleanVendorData", "vendorData")
}

// cleanField is a shared helper that applies a mutation function to one or all Hardware objects.
func cleanField(k8sClient client.Client, ctx context.Context, namespace, hostname string, mutate func(*tinkerbellKubeObjects.Hardware), funcName, fieldName string) error {
	if hostname != "" {
		hw := &tinkerbellKubeObjects.Hardware{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: hostname}, hw); err != nil {
			return fmt.Errorf("cannot get Hardware %s/%s: %w", namespace, hostname, err)
		}
		return patchField(k8sClient, ctx, hw, mutate, funcName, fieldName)
	}

	hwList := &tinkerbellKubeObjects.HardwareList{}
	if err := k8sClient.List(ctx, hwList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("cannot list Hardware in namespace %s: %w", namespace, err)
	}

	if len(hwList.Items) == 0 {
		slog.Warn("no Hardware objects found", "func", funcName, "namespace", namespace)
		return nil
	}

	for i := range hwList.Items {
		if err := patchField(k8sClient, ctx, &hwList.Items[i], mutate, funcName, fieldName); err != nil {
			return fmt.Errorf("failed to patch Hardware %s: %w", hwList.Items[i].Name, err)
		}
	}

	return nil
}

func patchField(k8sClient client.Client, ctx context.Context, hw *tinkerbellKubeObjects.Hardware, mutate func(*tinkerbellKubeObjects.Hardware), funcName, fieldName string) error {
	patch := client.MergeFrom(hw.DeepCopy())
	mutate(hw)

	if err := k8sClient.Patch(ctx, hw, patch); err != nil {
		return fmt.Errorf("cannot patch Hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	slog.Info("field cleared", "func", funcName, "field", fieldName, "name", hw.Name, "namespace", hw.Namespace)
	return nil
}
