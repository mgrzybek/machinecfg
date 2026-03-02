/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package tinkerbell

import (
	"context"
	"fmt"

	tinkerbellKubeObjects "github.com/tinkerbell/tink/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// GetHardwares returns one or all Hardware objects from a Kubernetes namespace.
// If hostname is empty, all Hardware objects in the namespace are returned.
func GetHardwares(k8sClient client.Client, ctx context.Context, namespace, hostname string) ([]tinkerbellKubeObjects.Hardware, error) {
	if hostname != "" {
		hw := &tinkerbellKubeObjects.Hardware{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: hostname}, hw); err != nil {
			return nil, fmt.Errorf("cannot get Hardware %s/%s: %w", namespace, hostname, err)
		}
		return []tinkerbellKubeObjects.Hardware{*hw}, nil
	}

	hwList := &tinkerbellKubeObjects.HardwareList{}
	if err := k8sClient.List(ctx, hwList, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("cannot list Hardware in namespace %s: %w", namespace, err)
	}

	return hwList.Items, nil
}
