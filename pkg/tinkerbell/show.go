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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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

// GetClusterName resolves the CAPI cluster name for a Hardware object by traversing
// the ownership chain: Hardware → TinkerbellMachine → cluster.x-k8s.io/cluster-name label.
// Returns an empty string if the Hardware has no Tinkerbell owner or the TinkerbellMachine
// cannot be found.
func GetClusterName(k8sClient client.Client, ctx context.Context, hw *tinkerbellKubeObjects.Hardware) string {
	ownerName := hw.Labels["v1alpha1.tinkerbell.org/ownerName"]
	ownerNamespace := hw.Labels["v1alpha1.tinkerbell.org/ownerNamespace"]

	if ownerName == "" || ownerNamespace == "" {
		return ""
	}

	tinkMachine := &unstructured.Unstructured{}
	tinkMachine.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "infrastructure.cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "TinkerbellMachine",
	})

	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: ownerNamespace, Name: ownerName}, tinkMachine); err != nil {
		slog.Debug("cannot get TinkerbellMachine", "func", "GetClusterName", "name", ownerName, "namespace", ownerNamespace, "error", err.Error())
		return ""
	}

	return tinkMachine.GetLabels()["cluster.x-k8s.io/cluster-name"]
}
