# MachineCFG Kubernetes Controllers

The controllers run inside the management cluster and automate the synchronisation between NetBox and Kubernetes — without manual CLI intervention.

| Controller | Direction | Trigger |
| --- | --- | --- |
| `controller-netbox-updater` | Kubernetes → NetBox | Hardware annotation change, CAPI Cluster change |
| `controller-kubernetes-updater` | NetBox → Kubernetes | Periodic poll (configurable interval) |

Both controllers support **hot config reload**: updating the ConfigMap or Secret takes effect immediately without a pod restart.

---

## 🚀 Installation

### Building images

```bash
# Create the OCI image
nix-build -A machinecfg-controller-netbox-updater-oci     -o machinecfg-controller-netbox-updater.tar.gz
nix-build -A machinecfg-controller-kubernetes-updater-oci -o machinecfg-controller-kubernetes-updater.tar.gz
# Create the OCI images’ SBOMs
nix-build -A machinecfg-controller-netbox-updater-oci-sbom     -o machinecfg-controller-netbox-updater-oci-sbom
nix-build -A machinecfg-controller-kubernetes-updater-oci-sbom -o machinecfg-controller-kubernetes-updater-oci-sbom
```

## controller-netbox-updater

Watches Tinkerbell `Hardware` objects and CAPI `Cluster` objects, then writes the observed state back to NetBox.

### Reconcilers

| Reconciler | Watches | Action |
| --- | --- | --- |
| `ConfigReconciler` | `netbox-updater-config` ConfigMap + `netbox-updater-secret` Secret | Reloads NetBox credentials and backend config at runtime |
| `HardwareReconciler` | `tinkerbell.org/v1alpha1/Hardware` | Transitions the matching NetBox device to `active` when `v1alpha1.tinkerbell.org/provisioned=true`; reverts to `staged` when the annotation is absent |
| `ClusterReconciler` | `cluster.x-k8s.io/v1beta1/Cluster` | Calls `cluster.SyncStatus`: ensures FHRP group, IP addresses (Cilium LB-IPAM and Tailscale), ServiceTemplate and Service exist in NetBox |

### Configuration

**ConfigMap `netbox-updater-config`**

| Key | Default | Description |
| --- | --- | --- |
| `netbox_endpoint` | `http://netbox.svc` | Base URL of the NetBox instance |
| `backend` | `tinkerbell` | Provisioning backend. Only `tinkerbell` is currently active; `metal3` is planned (Epic 8) |
| `otel_enabled` | `false` | Set to `"true"` to enable OpenTelemetry tracing |
| `otel_endpoint` | — | OTLP gRPC collector endpoint (e.g. `otel-collector.monitoring.svc:4317`) |

**Secret `netbox-updater-secret`**

| Key | Description |
| --- | --- |
| `netbox_token` | NetBox API token (40 characters) |

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `CONTROLLER_NAMESPACE` | `default` | Namespace where the controller reads its ConfigMap and Secret |
| `LEADER_ELECT` | `true` | Set to `"false"` to disable leader election (useful in local dev) |
| `LIMITS_CPU` | — | Injected by the Kubernetes Downward API; used to autotune `GOMAXPROCS` to the container CPU limit |

### Ports

| Port | Purpose |
| --- | --- |
| `:8080` | Prometheus metrics (`/metrics`) |
| `:8081` | Health probes (`/healthz`, `/readyz`) |

**Leader election ID**: `netbox-updater.machinecfg`

### Example manifests

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: netbox-updater-config
  namespace: tinkerbell
data:
  netbox_endpoint: https://netbox.example.com
  backend: tinkerbell
  otel_enabled: "false"
---
apiVersion: v1
kind: Secret
metadata:
  name: netbox-updater-secret
  namespace: tinkerbell
stringData:
  netbox_token: <your-40-char-token>
```

---

## controller-kubernetes-updater

Polls NetBox on a configurable interval and reconciles Tinkerbell `Hardware` objects to match the current device inventory.

### Reconcilers

| Reconciler | Watches | Action |
| --- | --- | --- |
| `ConfigReconciler` | `kubernetes-updater-config` ConfigMap + `kubernetes-updater-secret` Secret | Reloads credentials, filters and sync interval at runtime |
| `NetBoxSyncReconciler` | `kubernetes-updater-config` ConfigMap (trigger) + self-requeue via `RequeueAfter` | Fetches NetBox devices, creates or updates `Hardware` for `staged` devices, deletes `Hardware` for `offline`/`planned` devices |

### Configuration

**ConfigMap `kubernetes-updater-config`**

| Key | Default | Description |
| --- | --- | --- |
| `netbox_endpoint` | `http://netbox.svc` | Base URL of the NetBox instance |
| `backend` | `tinkerbell` | Provisioning backend. Only `tinkerbell` is currently active; `metal3` is planned (Epic 8) |
| `sync_interval` | `5m` | NetBox polling interval (Go duration format, e.g. `2m`, `30s`) |
| `ignition_variant` | — | When set (`flatcar` or `fcos`), embeds an Ignition config into the `userData` field of each `Hardware` object |
| `sites` | — | Comma-separated list of NetBox site names to include |
| `roles` | — | Comma-separated list of NetBox device role slugs to include |
| `tenants` | — | Comma-separated list of NetBox tenants (maps to Kubernetes namespaces) |
| `regions` | — | Comma-separated list of NetBox region names |
| `locations` | — | Comma-separated list of NetBox location names |
| `otel_enabled` | `false` | Set to `"true"` to enable OpenTelemetry tracing |
| `otel_endpoint` | — | OTLP gRPC collector endpoint |

**Secret `kubernetes-updater-secret`**

| Key | Description |
| --- | --- |
| `netbox_token` | NetBox API token (40 characters) |

### Environment variables

| Variable | Default | Description |
| --- | --- | --- |
| `CONTROLLER_NAMESPACE` | `default` | Namespace where the controller reads its ConfigMap and Secret |
| `LEADER_ELECT` | `true` | Set to `"false"` to disable leader election (useful in local dev) |
| `LIMITS_CPU` | — | Injected by the Kubernetes Downward API; used to autotune `GOMAXPROCS` to the container CPU limit |

### Ports

| Port | Purpose |
| --- | --- |
| `:8080` | Prometheus metrics (`/metrics`) |
| `:8081` | Health probes (`/healthz`, `/readyz`) |

**Leader election ID**: `kubernetes-updater.machinecfg`

### Example manifests

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubernetes-updater-config
  namespace: tinkerbell
data:
  netbox_endpoint: https://netbox.example.com
  backend: tinkerbell
  sync_interval: 5m
  ignition_variant: flatcar
  sites: paris-dc1
  roles: cattle
  tenants: my-tenant
  otel_enabled: "false"
---
apiVersion: v1
kind: Secret
metadata:
  name: kubernetes-updater-secret
  namespace: tinkerbell
stringData:
  netbox_token: <your-40-char-token>
```
