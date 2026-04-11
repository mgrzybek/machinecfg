# MachineCFG CLI

## 🚀 Installation

Using `task`:

```bash
task machinecfg
```

Using Nix:

```bash
# Create the OCI image
nix-build -A machinecfg-oci -o machinecfg.tar.gz
# Create the OCI image’s SBOMs
nix-build -A machinecfg-oci-sbom -o machinecfg-oci-sbom
```

## ⚙️ Configuration

### NetBox credentials

MachineCFG requires access to your NetBox instance. Credentials can be supplied via flags or environment variables (flags take precedence).

| Flag                 | Environment variable  | Description                                              |
|----------------------|-----------------------|----------------------------------------------------------|
| `--netbox-endpoint`  | `NETBOX_ENDPOINT`     | Base URL of your NetBox instance (e.g. `https://netbox.example.com`) |
| `--netbox-token`     | `NETBOX_TOKEN`        | NetBox API token (40 characters)                         |

### Kubernetes configuration

| Flag            | Environment variable | Description                                              |
|-----------------|----------------------|----------------------------------------------------------|
| `--kubeconfig`  | `KUBECONFIG`         | Path to kubeconfig file (flag overrides env var; falls back to `~/.kube/config`) |

## 🛠 Usage

| Command               | Description                                    |
|-----------------------|------------------------------------------------|
| `butane` / `ignition` | Manage Butane / Ignition configurations        |
| `talos`               | Manage Talos Linux machine config patches      |
| `tinkerbell`          | Manage Tinkerbell Hardware objects             |
| `cluster`             | Inspect and reconcile Kubernetes cluster state |

Global filter flags available on device commands (`tinkerbell`, `talos`, `butane`/`ignition`):

| Flag             | Description                                      |
|------------------|--------------------------------------------------|
| `--sites`        | Filter by NetBox site names (required)           |
| `--roles`        | Filter by NetBox device roles (required)         |
| `--regions`      | Filter by NetBox region names                    |
| `--locations`    | Filter by NetBox location names                  |
| `--tenants`      | Tenants / Kubernetes namespaces to scope commands to |
| `--namespaces`   | Alias for `--tenants`                            |
| `--racks`        | Filter by rack IDs                               |
| `--clusters`     | Filter by NetBox cluster names                   |
| `--virtualization` | Use virtual machines instead of physical devices |

---

### Tinkerbell Hardware

#### Sync Hardware objects

Create Hardware objects for `staged` devices and delete them for `offline`/`planned` devices.
When an object already exists, its labels are reconciled with the current NetBox state. If the
Hardware carries the `v1alpha1.tinkerbell.org/provisioned: "true"` annotation and the corresponding
NetBox device is still `staged`, the device is automatically transitioned to `active`.

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites paris-dc1 --roles cattle \
  tinkerbell hardware sync
```

Embed Fedora CoreOS vendor data:

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites paris-dc1 --roles cattle \
  tinkerbell hardware sync \
    --embed-ignition-as-vendor-data \
    --embedded-ignition-variant=fcos
```

Export as YAML files instead of applying to Kubernetes:

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites paris-dc1 --roles cattle \
  tinkerbell hardware sync \
    --output-directory /tmp
```

> Each generated `Hardware` object carries a `netbox-device-id` label for unambiguous traceability back to its NetBox source.

#### Show Hardware objects

Display all Hardware objects in a namespace with their PXE, Workflow and CAPI cluster membership:

```bash
./machinecfg tinkerbell hardware show \
    --tenants my-tenant
```

Example output:

```console
HOSTNAME           STATUS    ALLOW-PXE   WORKFLOW   CLUSTER
server-paris-01    staged    true        true
server-paris-02    active    false       false       cluster-0
server-paris-03    offline   false       false
```

The `CLUSTER` column is resolved by traversing the ownership chain
`Hardware → TinkerbellMachine → cluster.x-k8s.io/cluster-name`.

Filter to a single machine:

```bash
./machinecfg tinkerbell hardware show \
    --tenants my-tenant \
    --hostname server-paris-02
```

Output as JSON (logs are suppressed automatically):

```bash
./machinecfg tinkerbell hardware show \
    --tenants my-tenant \
    --output json | jq '.[].cluster'
```

#### Sync NetBox device status

Transition `staged` NetBox devices to `active` for all Hardware objects in a namespace
whose `v1alpha1.tinkerbell.org/provisioned` annotation is `"true"`.
Only devices that are currently `staged` are updated (`updated: true`).

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites paris-dc1 --roles cattle \
  tinkerbell hardware sync-status \
    --tenants my-tenant
```

Output as JSON:

```bash
./machinecfg ... tinkerbell hardware sync-status \
    --tenants my-tenant \
    --output json | jq '.[] | select(.updated)'
```

#### Enable / disable PXE boot

Enable PXE boot (`AllowPXE=true`) on all Hardware objects in a namespace:

```bash
./machinecfg tinkerbell hardware pxe-allow \
    --tenants my-tenant
```

Target a single machine:

```bash
./machinecfg tinkerbell hardware pxe-allow \
    --tenants my-tenant \
    --hostname my-server
```

The aliases `allow-pxe` and `deny-pxe` are also available for `pxe-allow` and `pxe-deny`.

#### Clean userData / vendorData

Wipe the `userData` field (e.g. after a reprovisioning cycle):

```bash
./machinecfg tinkerbell hardware clean-userdata \
    --tenants my-tenant

./machinecfg tinkerbell hardware clean-userdata \
    --tenants my-tenant \
    --hostname my-server
```

Wipe the `vendorData` field (forces a fresh embedded Ignition config on next provisioning):

```bash
./machinecfg tinkerbell hardware clean-vendordata \
    --tenants my-tenant
```

---

### Cluster

The `cluster` commands cross-reference NetBox Virtualization clusters with live CAPI / Kamaji
cluster state. They do **not** require `--sites` or `--roles`.

#### Show clusters

List all Kubernetes clusters with their NetBox status, CAPI readiness and member devices:

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
  cluster show \
    --tenants mushroomcloud
```

Example output:

```console
NAME          TYPE                    NETBOX-STATUS   CAPI-READY   DEVICE-COUNT   DEVICES
cluster-0     managed-kubernetes      active          true         1              cn-0
management    standalone-kubernetes   active                       1              management
```

Filter to a specific cluster:

```bash
./machinecfg ... cluster show \
    --tenants mushroomcloud \
    --clusters cluster-0
```

Output as JSON:

```bash
./machinecfg ... cluster show \
    --tenants mushroomcloud \
    --output json | jq '.[].devices'
```

#### Show cluster IP addresses

List the IP addresses advertised by each cluster and cross-reference them with NetBox IPAM.
Two sources are reported per cluster:

| Source | Description |
|---|---|
| `cilium-lb-ipam` | IP from the `io.cilium/lb-ipam-ips` annotation in `KamajiControlPlane.spec.network.serviceAnnotations` |
| `tailscale` | MagicDNS FQDN (or IP) from the Tailscale operator Secret, when the cluster is Tailscale-exposed |

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
  cluster ipaddr show \
    --tenants mushroomcloud
```

Example output:

```console
CLUSTER     IP-ADDRESS                   SOURCE           NETBOX-ASSIGNED   NETBOX-STATUS
cluster-0   192.168.3.8                  cilium-lb-ipam   true              active
cluster-0   cluster-0.tailxxxxx.ts.net   tailscale        false
```

Filter to a specific cluster:

```bash
./machinecfg ... cluster ipaddr show \
    --tenants mushroomcloud \
    --clusters cluster-0
```

#### Sync cluster status to NetBox

Write back cluster state to NetBox for each Kubernetes cluster. Two NetBox cluster types are supported:

| Type slug               | Control-plane source                         | Cilium IP / FHRP |
|-------------------------|----------------------------------------------|------------------|
| `managed-kubernetes`    | CAPI `Cluster` + `KamajiControlPlane`        | Yes              |
| `standalone-kubernetes` | Primary IP of the headnode DCIM device or VM | No               |

For each cluster the command ensures the following NetBox records exist:

- an **FHRP group** named after the cluster (protocol: other)
- for `managed-kubernetes`: the **Cilium LB-IPAM address** advertised by Cilium, assigned to that FHRP group
- for Tailscale-exposed clusters: the **Tailscale IP** registered in NetBox IPAM (a `/32` host prefix is created if no covering prefix exists), also assigned to the same FHRP group
- a **ServiceTemplate** `Kubernetes endpoint` (TCP, port from `controlPlaneEndpoint`)
- a **Service** attached to the FHRP group

Clusters of other types are silently skipped.

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
  cluster sync-status \
    --tenants mushroomcloud
```

Example output:

```console
CLUSTER     FHRP-GROUP-ID   IP-ADDRESS-ID   TAILSCALE-ADDRESS   SERVICE-ID   UPDATED   ERROR
cluster-0   1               13              1xx.xxx.xxx.xxx      1            true
```

Output as JSON (filter updated entries only):

```bash
./machinecfg ... cluster sync-status \
    --tenants mushroomcloud \
    --output json | jq '.[] | select(.updated)'
```

The command is **idempotent**: running it a second time leaves existing objects unchanged
(`updated: false`).

After `sync-status`, running `cluster ipaddr show` will report `NETBOX-ASSIGNED=true` for
any IP found in the `KamajiControlPlane` Cilium annotation.

##### DNS name resolution

The `dns_name` field of the IP address is populated using the following priority:

1. `spec.controlPlaneEndpoint.host` of the CAPI Cluster, **if it is a hostname** (not a bare IP).
2. Otherwise, the parent IPAM prefix's `Domains` custom field is read. The first entry that does
   **not** start with `~` (systemd-networkd routing-only prefix) is used to construct
   `<cluster-name>.<domain>`.

If neither source yields a valid hostname, `dns_name` is left empty.

---

### Create Ignition files

Write an `.ign` file per device into `/tmp`, using Flatcar variant:

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites maison --roles cattle \
  ignition flatcar \
    --output-directory /tmp
```

Using Fedora CoreOS variant:

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites maison --roles cattle \
  ignition fcos \
    --output-directory /tmp
```

---

### Create Talos Linux patches

```bash
./machinecfg \
    --netbox-endpoint $NETBOX_ENDPOINT --netbox-token $NETBOX_TOKEN \
    --sites paris-dc1 --roles cattle \
  talos machineconfig \
    --output-directory /tmp
```
