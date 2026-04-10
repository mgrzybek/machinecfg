/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"machinecfg/pkg/cluster"
)

// makeTailscaleStatefulSet builds an unstructured StatefulSet in the tailscale
// namespace carrying the parent-resource labels expected by the operator.
func makeTailscaleStatefulSet(name, clusterName, clusterNamespace string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "StatefulSet",
	})
	obj.SetName(name)
	obj.SetNamespace("tailscale")
	obj.SetLabels(map[string]string{
		"tailscale.com/parent-resource":    clusterName,
		"tailscale.com/parent-resource-ns": clusterNamespace,
	})
	return obj
}

// makeTailscaleSecret builds a corev1.Secret that mimics the per-pod state secret
// written by the Tailscale Kubernetes operator. The operator names it
// "<statefulset-name>-0" (pod index 0). Pass the StatefulSet name as ssName;
// the function appends the "-0" suffix automatically.
func makeTailscaleSecret(ssName, fqdn string, ips []string) *corev1.Secret {
	data := map[string][]byte{}
	if fqdn != "" {
		// Mirror real operator behaviour: FQDN is stored with a trailing DNS dot.
		data["device_fqdn"] = []byte(fqdn + ".")
	}
	if len(ips) > 0 {
		raw, _ := json.Marshal(ips)
		data["device_ips"] = raw
	}
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ssName + "-0",
			Namespace: "tailscale",
		},
		Data: data,
	}
}

// makeKamajiControlPlaneWithTailscale builds a KamajiControlPlane object with
// Tailscale annotations in spec.network.serviceAnnotations, which is the same
// field used for Cilium LB-IPAM annotations in the real operator.
func makeKamajiControlPlaneWithTailscale(name, namespace, tsHostname string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1alpha1",
		Kind:    "KamajiControlPlane",
	})
	obj.SetName(name)
	obj.SetNamespace(namespace)
	obj.Object["spec"] = map[string]interface{}{
		"network": map[string]interface{}{
			"serviceAnnotations": map[string]interface{}{
				"tailscale.com/expose":   "true",
				"tailscale.com/hostname": tsHostname,
			},
		},
	}
	return obj
}

// TestGetTailscaleDevice_FQDNPreferred verifies that device_fqdn takes priority over device_ips.
func TestGetTailscaleDevice_FQDNPreferred(t *testing.T) {
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	secret := makeTailscaleSecret("ts-my-cluster", "my-cluster.tailnet.ts.net", []string{"100.64.0.1"})
	k8sClient := fake.NewClientBuilder().WithObjects(ss, secret).Build()

	dev, err := cluster.GetTailscaleDevice(k8sClient, context.Background(), testClusterName, testNamespace)
	require.NoError(t, err)
	assert.Equal(t, "my-cluster.tailnet.ts.net", dev.FQDN)
	assert.Equal(t, "100.64.0.1", dev.IP)
	assert.Equal(t, "my-cluster.tailnet.ts.net", dev.Address())
}

// TestGetTailscaleDevice_IPFallback verifies that when device_fqdn is absent,
// the first IP from device_ips is used.
func TestGetTailscaleDevice_IPFallback(t *testing.T) {
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	secret := makeTailscaleSecret("ts-my-cluster", "", []string{"100.64.0.1"})
	k8sClient := fake.NewClientBuilder().WithObjects(ss, secret).Build()

	dev, err := cluster.GetTailscaleDevice(k8sClient, context.Background(), testClusterName, testNamespace)
	require.NoError(t, err)
	assert.Empty(t, dev.FQDN)
	assert.Equal(t, "100.64.0.1", dev.IP)
	assert.Equal(t, "100.64.0.1", dev.Address())
}

// TestGetTailscaleDevice_NoStatefulSet verifies that a missing StatefulSet returns an error.
func TestGetTailscaleDevice_NoStatefulSet(t *testing.T) {
	k8sClient := fake.NewClientBuilder().Build()

	_, err := cluster.GetTailscaleDevice(k8sClient, context.Background(), testClusterName, testNamespace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Tailscale StatefulSet found")
}

// TestGetTailscaleDevice_NoSecret verifies that a StatefulSet without a Secret returns an error.
func TestGetTailscaleDevice_NoSecret(t *testing.T) {
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	k8sClient := fake.NewClientBuilder().WithObjects(ss).Build()

	_, err := cluster.GetTailscaleDevice(k8sClient, context.Background(), testClusterName, testNamespace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot get Tailscale state Secret")
}

// TestGetTailscaleDevice_EmptySecret verifies that a Secret with no address data returns an error.
func TestGetTailscaleDevice_EmptySecret(t *testing.T) {
	ss := makeTailscaleStatefulSet("ts-my-cluster", testClusterName, testNamespace)
	secret := makeTailscaleSecret("ts-my-cluster", "", nil)
	k8sClient := fake.NewClientBuilder().WithObjects(ss, secret).Build()

	_, err := cluster.GetTailscaleDevice(k8sClient, context.Background(), testClusterName, testNamespace)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "contains no device address")
}

// TestIsTailscaleExposed_HappyPath verifies detection of a correctly annotated KCP.
func TestIsTailscaleExposed_HappyPath(t *testing.T) {
	kcp := makeKamajiControlPlaneWithTailscale(testClusterName, testNamespace, "my-cluster")
	exposed, hostname := cluster.IsTailscaleExposed(kcp)
	assert.True(t, exposed)
	assert.Equal(t, "my-cluster", hostname)
}

// TestIsTailscaleExposed_MissingExpose verifies that the absence of the expose annotation returns false.
func TestIsTailscaleExposed_MissingExpose(t *testing.T) {
	kcp := makeKamajiControlPlane(testClusterName, testNamespace, "")
	exposed, _ := cluster.IsTailscaleExposed(kcp)
	assert.False(t, exposed)
}

// TestIsTailscaleExposed_MissingHostname verifies that a missing hostname annotation returns false.
func TestIsTailscaleExposed_MissingHostname(t *testing.T) {
	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "controlplane.cluster.x-k8s.io",
		Version: "v1alpha1",
		Kind:    "KamajiControlPlane",
	})
	kcp.SetName(testClusterName)
	kcp.SetNamespace(testNamespace)
	kcp.Object["spec"] = map[string]interface{}{
		"network": map[string]interface{}{
			"serviceAnnotations": map[string]interface{}{
				"tailscale.com/expose": "true",
				// tailscale.com/hostname intentionally absent
			},
		},
	}
	exposed, _ := cluster.IsTailscaleExposed(kcp)
	assert.False(t, exposed)
}
