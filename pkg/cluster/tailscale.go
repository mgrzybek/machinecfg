/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/netbox-community/go-netbox/v4"
)

const (
	tailscaleExposeAnnotation      = "tailscale.com/expose"
	tailscaleHostnameAnnotation    = "tailscale.com/hostname"
	tailscaleNamespace             = "tailscale"
	tailscaleParentResourceLabel   = "tailscale.com/parent-resource"
	tailscaleParentResourceNsLabel = "tailscale.com/parent-resource-ns"
)

var statefulSetListGVK = schema.GroupVersionKind{
	Group:   "apps",
	Version: "v1",
	Kind:    "StatefulSetList",
}

// tailscaleDevice holds the Tailscale IP and MagicDNS FQDN of a cluster endpoint.
type tailscaleDevice struct {
	IP   string
	FQDN string
}

// Address returns the MagicDNS FQDN when available, falling back to the IP address.
// This is the preferred identifier for display and dns_name in NetBox.
func (d tailscaleDevice) Address() string {
	if d.FQDN != "" {
		return d.FQDN
	}
	return d.IP
}

// IsTailscaleExposed reports whether kcp (a KamajiControlPlane unstructured object)
// carries both tailscale.com/expose=true and a non-empty tailscale.com/hostname
// annotation in spec.network.serviceAnnotations (the same field used for Cilium
// LB-IPAM annotations). Returns (true, hostname) when both conditions are met.
func IsTailscaleExposed(kcp *unstructured.Unstructured) (bool, string) {
	annotations, _, _ := unstructured.NestedStringMap(
		kcp.Object,
		"spec", "network", "serviceAnnotations",
	)
	if annotations[tailscaleExposeAnnotation] != "true" {
		return false, ""
	}
	hostname := annotations[tailscaleHostnameAnnotation]
	return hostname != "", hostname
}

// GetTailscaleDevice locates the StatefulSet created by the Tailscale Kubernetes
// operator for the service named clusterName in clusterNamespace (matched via the
// tailscale.com/parent-resource and tailscale.com/parent-resource-ns labels in the
// tailscale namespace), then reads the MagicDNS FQDN ("device_fqdn") and first IP
// ("device_ips") from the co-located Secret.
//
// Returns an error if no StatefulSet is found or the Secret contains no address.
func GetTailscaleDevice(k8sClient client.Client, ctx context.Context, clusterName, clusterNamespace string) (tailscaleDevice, error) {
	ssList := &unstructured.UnstructuredList{}
	ssList.SetGroupVersionKind(statefulSetListGVK)

	if err := k8sClient.List(ctx, ssList,
		client.InNamespace(tailscaleNamespace),
		client.MatchingLabels{
			tailscaleParentResourceLabel:   clusterName,
			tailscaleParentResourceNsLabel: clusterNamespace,
		},
	); err != nil {
		return tailscaleDevice{}, fmt.Errorf("cannot list Tailscale StatefulSets for %s/%s: %w", clusterNamespace, clusterName, err)
	}

	if len(ssList.Items) == 0 {
		return tailscaleDevice{}, fmt.Errorf("no Tailscale StatefulSet found for %s/%s in namespace %s", clusterNamespace, clusterName, tailscaleNamespace)
	}

	ssName := ssList.Items[0].GetName()

	// The Tailscale operator names the per-pod state Secret "<statefulset>-<pod-index>".
	// Since Tailscale proxy StatefulSets always run a single replica, the pod index is 0.
	secretName := ssName + "-0"

	secret := &corev1.Secret{}
	if err := k8sClient.Get(ctx, client.ObjectKey{
		Namespace: tailscaleNamespace,
		Name:      secretName,
	}, secret); err != nil {
		return tailscaleDevice{}, fmt.Errorf("cannot get Tailscale state Secret %s/%s: %w", tailscaleNamespace, secretName, err)
	}

	var dev tailscaleDevice
	// Trim trailing DNS dot (e.g. "cluster-0.tail78d3a.ts.net." → "cluster-0.tail78d3a.ts.net")
	dev.FQDN = strings.TrimRight(strings.TrimSpace(string(secret.Data["device_fqdn"])), ".")

	if raw := secret.Data["device_ips"]; len(raw) > 0 {
		var ips []string
		if err := json.Unmarshal(raw, &ips); err == nil && len(ips) > 0 {
			dev.IP = ips[0]
		}
	}

	if dev.IP == "" && dev.FQDN == "" {
		return tailscaleDevice{}, fmt.Errorf("tailscale secret %s/%s contains no device address", tailscaleNamespace, ssName)
	}

	return dev, nil
}

// getKamajiTailscaleDevice reads the KamajiControlPlane for the given cluster and,
// when it is annotated for Tailscale exposure, returns the Tailscale device info.
// Returns (zero value, nil) when the cluster is not Tailscale-exposed or the KCP
// is not found, so callers can check dev.Address() == "" to skip Tailscale logic.
func getKamajiTailscaleDevice(k8sClient client.Client, ctx context.Context, namespace, name string) (tailscaleDevice, error) {
	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(kamajiControlPlaneGVK)

	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, kcp); err != nil {
		slog.Debug("KamajiControlPlane not found, skipping Tailscale check", "cluster", name, "error", err.Error())
		return tailscaleDevice{}, nil
	}

	exposed, _ := IsTailscaleExposed(kcp)
	if !exposed {
		return tailscaleDevice{}, nil
	}

	return GetTailscaleDevice(k8sClient, ctx, name, namespace)
}

// syncTailscaleIPAM records the Tailscale IP address in NetBox IPAM, creating a /32
// host prefix if no containing prefix exists. The device FQDN is used as dns_name.
// The IP is assigned to fhrpGroupID, the same group as the cluster's Cilium LB-IPAM IP.
func syncTailscaleIPAM(ctx context.Context, netboxClient *netbox.APIClient, fhrpGroupID int32, dev tailscaleDevice) (*netbox.IPAddress, bool, error) {
	if dev.IP == "" {
		return nil, false, fmt.Errorf("tailscale device has no IP address to sync to NetBox")
	}

	if err := getOrCreateIPAMPrefix(ctx, netboxClient, dev.IP); err != nil {
		slog.Warn("cannot ensure IPAM prefix for Tailscale IP", "ip", dev.IP, "error", err.Error())
	}

	return getOrCreateIPAddress(ctx, netboxClient, dev.IP, fhrpGroupID, dev.FQDN)
}

// getOrCreateIPAMPrefix ensures a prefix containing ip exists in NetBox IPAM.
// If none is found, a /32 host prefix is created for the address.
func getOrCreateIPAMPrefix(ctx context.Context, netboxClient *netbox.APIClient, ip string) error {
	existing, _, err := netboxClient.IpamAPI.IpamPrefixesList(ctx).
		Contains(ip).
		Execute()
	if err != nil {
		return fmt.Errorf("cannot query IPAM prefixes for %s: %w", ip, err)
	}

	if existing.Count > 0 {
		slog.Debug("IPAM prefix already contains Tailscale IP", "ip", ip)
		return nil
	}

	cidr := ip + "/32"
	req := netbox.NewWritablePrefixRequest(cidr)
	_, _, err = netboxClient.IpamAPI.IpamPrefixesCreate(ctx).
		WritablePrefixRequest(*req).
		Execute()
	if err != nil {
		return fmt.Errorf("cannot create IPAM prefix %s: %w", cidr, err)
	}

	slog.Info("IPAM prefix created for Tailscale IP", "prefix", cidr)
	return nil
}
