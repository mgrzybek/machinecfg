/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/netbox-community/go-netbox/v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ciliumLBIPAnnotation = "io.cilium/lb-ipam-ips"

var kamajiControlPlaneGVK = schema.GroupVersionKind{
	Group:   "controlplane.cluster.x-k8s.io",
	Version: "v1alpha1",
	Kind:    "KamajiControlPlane",
}

// IPAddressRow holds the combined Kubernetes + NetBox view of a single cluster IP.
type IPAddressRow struct {
	ClusterName    string `json:"cluster-name"`
	IPAddress      string `json:"ip-address"`
	Source         string `json:"source,omitempty"`
	NetBoxAssigned bool   `json:"netbox-assigned"`
	NetBoxStatus   string `json:"netbox-status,omitempty"`
}

// ShowIPAddresses lists the IP addresses advertised by Cilium LB-IPAM for each
// KamajiControlPlane object in the given namespace (filtered by clusterNames when
// non-empty) and enriches each address with its NetBox IPAM status. For clusters
// that are Tailscale-exposed, the Tailscale device address is appended as an
// additional row with source="tailscale".
//
// The Cilium IP is read from the annotation io.cilium/lb-ipam-ips in
// spec.network.serviceAnnotations of each KamajiControlPlane. Multiple IPs can
// be listed as a comma-separated value.
func ShowIPAddresses(
	k8sClient client.Client,
	ctx context.Context,
	namespace string,
	netboxClient *netbox.APIClient,
	clusterNames []string,
) ([]IPAddressRow, error) { //nolint:cyclop
	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   kamajiControlPlaneGVK.Group,
		Version: kamajiControlPlaneGVK.Version,
		Kind:    kamajiControlPlaneGVK.Kind + "List",
	})

	if err := k8sClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
		return nil, fmt.Errorf("cannot list KamajiControlPlane in namespace %s: %w", namespace, err)
	}

	if len(list.Items) == 0 {
		slog.Warn("no KamajiControlPlane objects found", "func", "ShowIPAddresses", "namespace", namespace)
		return nil, nil
	}

	clusterFilter := make(map[string]struct{}, len(clusterNames))
	for _, n := range clusterNames {
		if n != "" {
			clusterFilter[n] = struct{}{}
		}
	}

	var rows []IPAddressRow

	for _, obj := range list.Items {
		name := obj.GetName()

		if len(clusterFilter) > 0 {
			if _, ok := clusterFilter[name]; !ok {
				continue
			}
		}

		// Cilium LB-IPAM rows
		annotations, _, err := unstructured.NestedStringMap(obj.Object, "spec", "network", "serviceAnnotations")
		if err != nil {
			slog.Warn("cannot read serviceAnnotations", "func", "ShowIPAddresses", "cluster", name, "error", err.Error())
		} else {
			rawIPs := annotations[ciliumLBIPAnnotation]
			if rawIPs == "" {
				slog.Debug("no Cilium LB-IPAM annotation found", "func", "ShowIPAddresses", "cluster", name)
			}
			for _, ip := range strings.Split(rawIPs, ",") {
				ip = strings.TrimSpace(ip)
				if ip == "" {
					continue
				}
				assigned, status, ipErr := getNetBoxIPStatus(ctx, netboxClient, ip)
				if ipErr != nil {
					slog.Warn("cannot check NetBox IP status", "func", "ShowIPAddresses", "cluster", name, "ip", ip, "error", ipErr.Error())
				}
				rows = append(rows, IPAddressRow{
					ClusterName:    name,
					IPAddress:      ip,
					Source:         "cilium-lb-ipam",
					NetBoxAssigned: assigned,
					NetBoxStatus:   status,
				})
			}
		}

		// Tailscale endpoint (if the KamajiControlPlane is annotated for exposure)
		if exposed, _ := IsTailscaleExposed(&obj); exposed {
			dev, tsErr := GetTailscaleDevice(k8sClient, ctx, name, namespace)
			if tsErr != nil {
				slog.Warn("cannot get Tailscale device address", "func", "ShowIPAddresses", "cluster", name, "error", tsErr.Error())
			} else {
				var assigned bool
				var tsStatus string
				if dev.IP != "" {
					assigned, tsStatus, tsErr = getNetBoxIPStatus(ctx, netboxClient, dev.IP)
					if tsErr != nil {
						slog.Warn("cannot check NetBox IP status", "func", "ShowIPAddresses", "cluster", name, "ip", dev.IP, "error", tsErr.Error())
					}
				}
				rows = append(rows, IPAddressRow{
					ClusterName:    name,
					IPAddress:      dev.Address(),
					Source:         "tailscale",
					NetBoxAssigned: assigned,
					NetBoxStatus:   tsStatus,
				})
			}
		}
	}

	return rows, nil
}

// getNetBoxIPStatus checks whether the given IP address is known to NetBox IPAM.
// Returns (true, statusValue, nil) when found, (false, "", nil) when absent, or
// (false, "", err) on API failure.
func getNetBoxIPStatus(ctx context.Context, netboxClient *netbox.APIClient, ip string) (bool, string, error) {
	result, _, err := netboxClient.IpamAPI.IpamIpAddressesList(ctx).
		Q(ip).
		Execute()
	if err != nil {
		return false, "", fmt.Errorf("cannot query NetBox IPAM for %s: %w", ip, err)
	}

	if result.Count == 0 {
		return false, "", nil
	}

	ipStatus := result.Results[0].GetStatus()
	status := string(ipStatus.GetValue())
	return true, status, nil
}
