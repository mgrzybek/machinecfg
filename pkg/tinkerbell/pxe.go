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

// AllowPXE sets AllowPXE=true on one or all Hardware objects in a namespace.
// If hostname is empty, all Hardware objects in the namespace are patched.
// On error, the function stops immediately and returns the error.
func AllowPXE(k8sClient client.Client, ctx context.Context, namespace, hostname string) error {
	if hostname != "" {
		hw := &tinkerbellKubeObjects.Hardware{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: hostname}, hw); err != nil {
			return fmt.Errorf("cannot get Hardware %s/%s: %w", namespace, hostname, err)
		}
		return patchAllowPXE(k8sClient, ctx, hw)
	}

	hwList := &tinkerbellKubeObjects.HardwareList{}
	if err := k8sClient.List(ctx, hwList, client.InNamespace(namespace)); err != nil {
		return fmt.Errorf("cannot list Hardware in namespace %s: %w", namespace, err)
	}

	if len(hwList.Items) == 0 {
		slog.Warn("no Hardware objects found", "func", "AllowPXE", "namespace", namespace)
		return nil
	}

	for i := range hwList.Items {
		if err := patchAllowPXE(k8sClient, ctx, &hwList.Items[i]); err != nil {
			return fmt.Errorf("failed to patch Hardware %s: %w", hwList.Items[i].Name, err)
		}
	}

	return nil
}

// patchAllowPXE sets AllowPXE=true on all interfaces of a Hardware object.
// If an interface has no Netboot section, one is created.
func patchAllowPXE(k8sClient client.Client, ctx context.Context, hw *tinkerbellKubeObjects.Hardware) error {
	patch := client.MergeFrom(hw.DeepCopy())

	trueVal := true
	for i := range hw.Spec.Interfaces {
		if hw.Spec.Interfaces[i].Netboot == nil {
			slog.Debug("creating missing Netboot section", "func", "patchAllowPXE", "name", hw.Name, "interface", i)
			hw.Spec.Interfaces[i].Netboot = &tinkerbellKubeObjects.Netboot{}
		}
		hw.Spec.Interfaces[i].Netboot.AllowPXE = &trueVal
	}

	if err := k8sClient.Patch(ctx, hw, patch); err != nil {
		return fmt.Errorf("cannot patch Hardware %s/%s: %w", hw.Namespace, hw.Name, err)
	}

	slog.Info("AllowPXE set to true", "func", "patchAllowPXE", "name", hw.Name, "namespace", hw.Namespace)
	return nil
}
