/*
Copyright © 2025 Mathieu Grzybek <github@grzybek.fr>
SPDX-License-Identifier: GPL-3.0-or-later
*/
package cluster

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"

	"github.com/netbox-community/go-netbox/v4"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const serviceTemplateName = "Kubernetes endpoint"
const defaultControlPlanePort = int32(6443)
const managedKubernetesClusterTypeSlug = "managed-kubernetes"
const standaloneKubernetesClusterTypeSlug = "standalone-kubernetes"
const headnodeRoleSlug = "headnode"

// SyncStatusResult holds the outcome of a single cluster sync operation.
type SyncStatusResult struct {
	ClusterName      string `json:"cluster-name"`
	FHRPGroupID      int32  `json:"fhrp-group-id,omitempty"`
	ServiceID        int32  `json:"service-id,omitempty"`
	IPAddressID      int32  `json:"ip-address-id,omitempty"`
	TailscaleAddress string `json:"tailscale-address,omitempty"`
	Updated          bool   `json:"updated"`
	Error            string `json:"error,omitempty"`
}

// SyncStatus reads NetBox virtualization clusters of types managed-kubernetes and
// standalone-kubernetes (filtered by clusterNames) and, for each one, ensures that
// the corresponding FHRP group, ServiceTemplate and Service records exist in NetBox.
//
// For managed-kubernetes clusters the control-plane endpoint is read from the CAPI
// Cluster object in Kubernetes; for standalone-kubernetes clusters it is derived
// from the primary IP of the device with role "headnode" in the same NetBox cluster.
//
// A per-cluster error is recorded in the result instead of aborting so that the
// function continues processing the remaining clusters.
func SyncStatus(
	k8sClient client.Client,
	ctx context.Context,
	namespace string,
	netboxClient *netbox.APIClient,
	clusterNames []string,
) ([]SyncStatusResult, error) {
	req := netboxClient.VirtualizationAPI.VirtualizationClustersList(ctx).
		Type_([]string{managedKubernetesClusterTypeSlug, standaloneKubernetesClusterTypeSlug})
	if len(clusterNames) > 0 && clusterNames[0] != "" {
		req = req.Name(clusterNames)
	}

	netboxClusters, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("cannot list NetBox clusters: %w", err)
	}

	if netboxClusters.Count == 0 {
		slog.Warn("no NetBox clusters found", "func", "SyncStatus")
		return nil, nil
	}

	nextID, err := getNextFHRPGroupID(ctx, netboxClient)
	if err != nil {
		return nil, fmt.Errorf("cannot determine next FHRP group ID: %w", err)
	}

	var results []SyncStatusResult

	for _, nbCluster := range netboxClusters.Results {
		result := SyncStatusResult{ClusterName: nbCluster.Name}
		typeSlug := nbCluster.Type.GetSlug()

		var host string
		var port int32
		switch typeSlug {
		case managedKubernetesClusterTypeSlug:
			host, port, err = getCAPIClusterEndpoint(k8sClient, ctx, namespace, nbCluster.Name)
			if err != nil {
				result.Error = err.Error()
				slog.Warn("cannot get CAPI Cluster", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
				results = append(results, result)
				continue
			}
		case standaloneKubernetesClusterTypeSlug:
			host, port, err = getHeadnodeEndpoint(ctx, netboxClient, nbCluster.Id)
			if err != nil {
				result.Error = err.Error()
				slog.Warn("cannot get headnode endpoint", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
				results = append(results, result)
				continue
			}
		default:
			continue
		}

		fhrpGroup, created, err := getOrCreateFHRPGroup(ctx, netboxClient, nbCluster.Name, nextID)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create FHRP group", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if created {
			nextID++
			result.Updated = true
		}
		result.FHRPGroupID = fhrpGroup.Id

		if typeSlug == managedKubernetesClusterTypeSlug {
			ciliumIP, err := getKamajiControlPlaneIP(k8sClient, ctx, namespace, nbCluster.Name)
			if err != nil {
				slog.Warn("cannot get Cilium LB-IPAM IP from KamajiControlPlane", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			} else if ciliumIP != "" {
				dnsName := host
				if !isHostname(dnsName) {
					dnsName, _ = getDNSNameFromPrefix(ctx, netboxClient, ciliumIP, nbCluster.Name)
				}
				ipAddr, ipCreated, err := getOrCreateIPAddress(ctx, netboxClient, ciliumIP, fhrpGroup.Id, dnsName)
				if err != nil {
					result.Error = err.Error()
					slog.Error("cannot create IP address", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
					results = append(results, result)
					continue
				}
				if ipCreated {
					result.Updated = true
				}
				result.IPAddressID = ipAddr.GetId()
			}

			// Tailscale endpoint
			tsDev, tsErr := getKamajiTailscaleDevice(k8sClient, ctx, namespace, nbCluster.Name)
			if tsErr != nil {
				slog.Warn("cannot get Tailscale device info", "func", "SyncStatus", "cluster", nbCluster.Name, "error", tsErr.Error())
			} else if tsDev.Address() != "" {
				tsIPAddr, tsCreated, tsIPErr := syncTailscaleIPAM(ctx, netboxClient, fhrpGroup.Id, tsDev)
				if tsIPErr != nil {
					slog.Warn("cannot sync Tailscale IP to NetBox IPAM", "func", "SyncStatus", "cluster", nbCluster.Name, "error", tsIPErr.Error())
				} else {
					result.TailscaleAddress = tsDev.Address()
					if tsCreated {
						result.Updated = true
					}
					slog.Info("Tailscale IP synced to NetBox IPAM", "func", "SyncStatus", "cluster", nbCluster.Name, "ip", tsIPAddr.GetAddress())
				}
			}
		}

		_, templateCreated, err := getOrCreateServiceTemplate(ctx, netboxClient, port)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create ServiceTemplate", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if templateCreated {
			result.Updated = true
		}

		svc, svcCreated, err := getOrCreateService(ctx, netboxClient, fhrpGroup.Id, nbCluster.Name, port)
		if err != nil {
			result.Error = err.Error()
			slog.Error("cannot create Service", "func", "SyncStatus", "cluster", nbCluster.Name, "error", err.Error())
			results = append(results, result)
			continue
		}
		if svcCreated {
			result.Updated = true
		}
		result.ServiceID = svc.GetId()

		results = append(results, result)
	}

	return results, nil
}

// getCAPIClusterEndpoint looks up the CAPI Cluster object in the given namespace
// and returns its controlPlaneEndpoint host and port. The port falls back to
// defaultControlPlanePort if absent or zero; the host is empty string if absent.
func getCAPIClusterEndpoint(k8sClient client.Client, ctx context.Context, namespace, name string) (host string, port int32, err error) {
	capiCluster := &unstructured.Unstructured{}
	capiCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "cluster.x-k8s.io",
		Version: "v1beta1",
		Kind:    "Cluster",
	})

	if err = k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, capiCluster); err != nil {
		return "", 0, fmt.Errorf("cannot get CAPI Cluster %s/%s: %w", namespace, name, err)
	}

	host, _, _ = unstructured.NestedString(capiCluster.Object, "spec", "controlPlaneEndpoint", "host")

	portVal, found, portErr := unstructured.NestedInt64(capiCluster.Object, "spec", "controlPlaneEndpoint", "port")
	if portErr != nil || !found || portVal == 0 {
		slog.Debug("controlPlaneEndpoint.port not found, using default", "func", "getCAPIClusterEndpoint", "cluster", name)
		return host, defaultControlPlanePort, nil
	}

	return host, int32(portVal), nil
}

// getHeadnodeEndpoint returns the control-plane host and port for a
// standalone-kubernetes cluster. It first looks for a DCIM device with role
// "headnode" in the cluster; if none is found it falls back to looking for a
// virtual machine with the same role. The port is always defaultControlPlanePort.
func getHeadnodeEndpoint(ctx context.Context, netboxClient *netbox.APIClient, clusterID int32) (host string, port int32, err error) {
	result, _, err := netboxClient.DcimAPI.DcimDevicesList(ctx).
		ClusterId([]*int32{&clusterID}).
		Role([]string{headnodeRoleSlug}).
		Execute()
	if err != nil {
		return "", 0, fmt.Errorf("cannot list headnode devices in cluster %d: %w", clusterID, err)
	}

	if result.Count > 0 {
		device := result.Results[0]

		var addr string
		if device.HasPrimaryIp4() {
			ip4 := device.GetPrimaryIp4()
			addr = ip4.GetAddress()
		} else if device.HasPrimaryIp() {
			ip := device.GetPrimaryIp()
			addr = ip.GetAddress()
		}

		if addr == "" {
			return "", 0, fmt.Errorf("headnode device %q has no primary IP", device.GetName())
		}

		// Strip CIDR suffix (e.g. "192.168.1.10/24" → "192.168.1.10")
		return strings.SplitN(addr, "/", 2)[0], defaultControlPlanePort, nil
	}

	// No DCIM device found — try virtual machines.
	host, err = getHeadnodeVMIP(ctx, netboxClient, clusterID)
	if err != nil {
		return "", 0, err
	}
	return host, defaultControlPlanePort, nil
}

// getHeadnodeVMIP returns the primary IPv4 address (CIDR suffix stripped) of the
// first virtual machine with role "headnode" in the given NetBox cluster.
func getHeadnodeVMIP(ctx context.Context, netboxClient *netbox.APIClient, clusterID int32) (string, error) {
	result, _, err := netboxClient.VirtualizationAPI.VirtualizationVirtualMachinesList(ctx).
		ClusterId([]*int32{&clusterID}).
		Role([]string{headnodeRoleSlug}).
		Execute()
	if err != nil {
		return "", fmt.Errorf("cannot list headnode VMs in cluster %d: %w", clusterID, err)
	}

	if result.Count == 0 {
		return "", fmt.Errorf("no headnode device or VM found in cluster %d", clusterID)
	}

	vm := result.Results[0]

	var addr string
	if vm.HasPrimaryIp4() {
		ip4 := vm.GetPrimaryIp4()
		addr = ip4.GetAddress()
	} else if vm.HasPrimaryIp() {
		ip := vm.GetPrimaryIp()
		addr = ip.GetAddress()
	}

	if addr == "" {
		return "", fmt.Errorf("headnode VM %q has no primary IP", vm.GetName())
	}

	return strings.SplitN(addr, "/", 2)[0], nil
}

// getNextFHRPGroupID returns max(group_id)+1 across all existing FHRP groups,
// or 1 if none exist.
func getNextFHRPGroupID(ctx context.Context, netboxClient *netbox.APIClient) (int32, error) {
	result, _, err := netboxClient.IpamAPI.IpamFhrpGroupsList(ctx).
		Limit(1).
		Ordering("-group_id").
		Execute()
	if err != nil {
		return 0, fmt.Errorf("cannot list FHRP groups: %w", err)
	}

	if result.Count == 0 {
		return 1, nil
	}

	return result.Results[0].GetGroupId() + 1, nil
}

// getOrCreateFHRPGroup looks up a FHRP group by name. If none is found it
// creates one with protocol=other and the given groupID. Returns the group,
// a boolean indicating whether it was created, and any error.
func getOrCreateFHRPGroup(ctx context.Context, netboxClient *netbox.APIClient, name string, groupID int32) (*netbox.FHRPGroup, bool, error) {
	existing, _, err := netboxClient.IpamAPI.IpamFhrpGroupsList(ctx).
		Name([]string{name}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list FHRP groups by name %q: %w", name, err)
	}

	if existing.Count > 0 {
		slog.Debug("FHRP group already exists", "func", "getOrCreateFHRPGroup", "name", name)
		return &existing.Results[0], false, nil
	}

	req := netbox.NewFHRPGroupRequest(netbox.BRIEFFHRPGROUPPROTOCOL_OTHER, groupID)
	req.SetName(name)

	created, _, err := netboxClient.IpamAPI.IpamFhrpGroupsCreate(ctx).
		FHRPGroupRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create FHRP group %q: %w", name, err)
	}

	slog.Info("FHRP group created", "func", "getOrCreateFHRPGroup", "name", name, "group_id", groupID)
	return created, true, nil
}

// getOrCreateServiceTemplate looks up the "Kubernetes endpoint" service template.
// If not found it creates it with TCP protocol and the given port.
func getOrCreateServiceTemplate(ctx context.Context, netboxClient *netbox.APIClient, port int32) (*netbox.ServiceTemplate, bool, error) {
	existing, _, err := netboxClient.IpamAPI.IpamServiceTemplatesList(ctx).
		Name([]string{serviceTemplateName}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list ServiceTemplates: %w", err)
	}

	if existing.Count > 0 {
		slog.Debug("ServiceTemplate already exists", "func", "getOrCreateServiceTemplate")
		return &existing.Results[0], false, nil
	}

	req := netbox.NewWritableServiceTemplateRequest(
		serviceTemplateName,
		netbox.PATCHEDWRITABLESERVICEREQUESTPROTOCOL_TCP,
		[]int32{port},
	)

	created, _, err := netboxClient.IpamAPI.IpamServiceTemplatesCreate(ctx).
		WritableServiceTemplateRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create ServiceTemplate: %w", err)
	}

	slog.Info("ServiceTemplate created", "func", "getOrCreateServiceTemplate", "port", port)
	return created, true, nil
}

// getKamajiControlPlaneIP reads the Cilium LB-IPAM IP from a KamajiControlPlane
// object's spec.network.serviceAnnotations["io.cilium/lb-ipam-ips"]. Returns
// the first IP found (trimmed), or an empty string if the annotation is absent.
func getKamajiControlPlaneIP(k8sClient client.Client, ctx context.Context, namespace, name string) (string, error) {
	kcp := &unstructured.Unstructured{}
	kcp.SetGroupVersionKind(kamajiControlPlaneGVK)

	if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, kcp); err != nil {
		return "", fmt.Errorf("cannot get KamajiControlPlane %s/%s: %w", namespace, name, err)
	}

	annotations, _, err := unstructured.NestedStringMap(kcp.Object, "spec", "network", "serviceAnnotations")
	if err != nil {
		return "", fmt.Errorf("cannot read serviceAnnotations from KamajiControlPlane %s: %w", name, err)
	}

	rawIPs, ok := annotations[ciliumLBIPAnnotation]
	if !ok || rawIPs == "" {
		return "", nil
	}

	// Return the first IP (comma-separated list)
	parts := strings.SplitN(rawIPs, ",", 2)
	return strings.TrimSpace(parts[0]), nil
}

// getDNSNameFromPrefix looks up the most specific IPAM prefix containing ip in
// NetBox and reads its Domains custom field. It returns "<clusterName>.<domain>"
// using the first domain found, or an empty string if the field is absent.
func getDNSNameFromPrefix(ctx context.Context, netboxClient *netbox.APIClient, ip, clusterName string) (string, error) {
	result, _, err := netboxClient.IpamAPI.IpamPrefixesList(ctx).
		Contains(ip).
		Ordering("-mask_length").
		Limit(1).
		Execute()
	if err != nil {
		return "", fmt.Errorf("cannot query NetBox prefixes for %s: %w", ip, err)
	}

	if result.Count == 0 {
		slog.Debug("no parent prefix found for IP", "func", "getDNSNameFromPrefix", "ip", ip)
		return "", nil
	}

	cf := result.Results[0].GetCustomFields()
	raw, ok := cf["Domains"]
	if !ok || raw == nil {
		slog.Debug("no Domains custom field on parent prefix", "func", "getDNSNameFromPrefix", "ip", ip)
		return "", nil
	}

	domainsStr, ok := raw.(string)
	if !ok || domainsStr == "" {
		return "", nil
	}

	// Domains is space-separated (systemd-networkd convention).
	// Entries prefixed with "~" are routing-only domains and are not valid DNS names.
	for _, domain := range strings.Fields(domainsStr) {
		if !strings.HasPrefix(domain, "~") {
			return clusterName + "." + domain, nil
		}
	}

	slog.Debug("no usable domain found in prefix Domains field (all are routing-only)", "func", "getDNSNameFromPrefix", "ip", ip, "domains", domainsStr)
	return "", nil
}

// isHostname reports whether s is a DNS hostname rather than a bare IP address.
// Only hostnames are valid for the NetBox dns_name field.
func isHostname(s string) bool {
	return s != "" && net.ParseIP(s) == nil
}

// getOrCreateIPAddress ensures the given IP address exists in NetBox IPAM and
// is assigned to the specified FHRP group. The IP may be given with or without
// a CIDR prefix (e.g. "192.168.3.8" or "192.168.3.8/32").
//
// If dnsName is a valid hostname (not a bare IP), it is written to dns_name on
// creation. If the IP already exists but has no dns_name, a PATCH is issued.
//
// Returns the IP address object, whether it was created or updated, and any error.
func getOrCreateIPAddress(ctx context.Context, netboxClient *netbox.APIClient, ip string, fhrpGroupID int32, dnsName string) (*netbox.IPAddress, bool, error) {
	existing, _, err := netboxClient.IpamAPI.IpamIpAddressesList(ctx).
		Q(ip).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot query NetBox IPAM for %s: %w", ip, err)
	}

	if existing.Count > 0 {
		addr := existing.Results[0]
		slog.Debug("IP address already exists in NetBox IPAM", "func", "getOrCreateIPAddress", "ip", ip)

		if isHostname(dnsName) && addr.GetDnsName() == "" {
			patch := netbox.NewPatchedWritableIPAddressRequest()
			patch.SetDnsName(dnsName)
			updated, _, err := netboxClient.IpamAPI.IpamIpAddressesPartialUpdate(ctx, addr.GetId()).
				PatchedWritableIPAddressRequest(*patch).
				Execute()
			if err != nil {
				slog.Warn("cannot set dns_name on IP address", "func", "getOrCreateIPAddress", "ip", ip, "dns_name", dnsName, "error", err.Error())
				return &addr, false, nil
			}
			slog.Info("dns_name set on existing IP address", "func", "getOrCreateIPAddress", "ip", ip, "dns_name", dnsName)
			return updated, true, nil
		}
		return &addr, false, nil
	}

	cidr := ip
	if !strings.Contains(ip, "/") {
		cidr = ip + "/32"
	}

	req := netbox.NewWritableIPAddressRequest(cidr)
	status := netbox.PATCHEDWRITABLEIPADDRESSREQUESTSTATUS_ACTIVE
	req.SetStatus(status)
	req.SetAssignedObjectType("ipam.fhrpgroup")
	req.SetAssignedObjectId(int64(fhrpGroupID))
	if isHostname(dnsName) {
		req.SetDnsName(dnsName)
	}

	created, _, err := netboxClient.IpamAPI.IpamIpAddressesCreate(ctx).
		WritableIPAddressRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create NetBox IP address %s: %w", cidr, err)
	}

	slog.Info("IP address created in NetBox IPAM", "func", "getOrCreateIPAddress", "ip", cidr, "fhrp_group_id", fhrpGroupID, "dns_name", dnsName)
	return created, true, nil
}

// getOrCreateService looks up a Service by its expected description. If not
// found it creates one attached to the given FHRP group.
func getOrCreateService(
	ctx context.Context,
	netboxClient *netbox.APIClient,
	fhrpGroupID int32,
	clusterName string,
	port int32,
) (*netbox.Service, bool, error) {
	description := fmt.Sprintf("Kubernetes endpoint for %s", clusterName)

	existing, _, err := netboxClient.IpamAPI.IpamServicesList(ctx).
		Description([]string{description}).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot list Services: %w", err)
	}

	if existing.Count > 0 {
		slog.Debug("Service already exists", "func", "getOrCreateService", "cluster", clusterName)
		svc := existing.Results[0]
		return &svc, false, nil
	}

	req := netbox.NewWritableServiceRequest(
		"ipam.fhrpgroup",
		int64(fhrpGroupID),
		serviceTemplateName,
		netbox.PATCHEDWRITABLESERVICEREQUESTPROTOCOL_TCP,
		[]int32{port},
	)
	req.SetDescription(description)

	created, _, err := netboxClient.IpamAPI.IpamServicesCreate(ctx).
		WritableServiceRequest(*req).
		Execute()
	if err != nil {
		return nil, false, fmt.Errorf("cannot create Service for cluster %q: %w", clusterName, err)
	}

	slog.Info("Service created", "func", "getOrCreateService", "cluster", clusterName, "fhrp_group_id", fhrpGroupID)
	return created, true, nil
}
