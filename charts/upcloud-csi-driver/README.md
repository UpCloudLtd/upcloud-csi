# UpCloud CSI Driver

Helm chart for the Kubernetes deployment in [`deploy/kubernetes`](../../deploy/kubernetes).
Resource names, RBAC, and storage classes are based on those manifests. The chart runs separate node and controller driver modes, with pinned images and a Recreate controller strategy.

The chart installs:

- CSIDriver `storage.csi.upcloud.com`
- Controller Deployment with the CSI snapshotter sidecar, and node DaemonSet
- Service accounts and RBAC
- StorageClasses and a VolumeSnapshotClass, unless disabled
- Optionally, the historical shared snapshot-controller Deployment and its RBAC

Install it into the namespace that contains the UpCloud credentials Secret. The static manifests use `kube-system`.

UpCloud Managed Kubernetes (UKS) already includes this driver. Skip this chart on UKS clusters.

## Prerequisites

- Kubernetes 1.20 or newer. The bundled snapshotter v4.2.1 and provisioner v3.1.0 require at least 1.20; the provisioner recommends 1.22.
- Linux AMD64 nodes for the default driver image; the v1.5.1 published image is AMD64-only.
- Helm 3 or newer
- A pre-existing Kubernetes Secret named `upcloud-csi-credentials`, containing either `username` and `password` or `token`.

```bash
kubectl -n kube-system create secret generic upcloud-csi-credentials \
  --from-literal=username="$UPCLOUD_USERNAME" \
  --from-literal=password="$UPCLOUD_PASSWORD"
```

To use an existing Secret in the release namespace, set `credentials.secretName` to its name.

### Snapshot infrastructure

Snapshots require the `snapshot.storage.k8s.io/v1` API and a compatible
cluster-wide snapshot controller. These are normally installed once per cluster
by the administrator or Kubernetes distribution, independently of any CSI driver.
The chart does not install or upgrade the CRDs. Check for all three CRDs and
inspect the cluster's deployments for an existing snapshot controller:

```bash
kubectl get crd volumesnapshots.snapshot.storage.k8s.io \
  volumesnapshotcontents.snapshot.storage.k8s.io \
  volumesnapshotclasses.snapshot.storage.k8s.io -o yaml
kubectl get deployments -A -o wide | grep snapshot-controller
```

Confirm that each CRD has a `v1` version with `served: true` and that the
existing controller is healthy and compatible. The Deployment search is a
starting point: distributions may use a different name or manage the
controller outside a Deployment.

If components are missing, the following example uses the official
[external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter)
v4.2.1 release, matching the chart's CSI snapshotter sidecar. Check compatibility
with your Kubernetes distribution and any installed snapshot components first.
Apply **only the missing CRDs**, then install the shared controller only if no
compatible controller already exists:

```bash
SNAPSHOTTER=https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/v4.2.1
# Run each line only if that CRD is missing.
kubectl apply -f "$SNAPSHOTTER/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"
kubectl apply -f "$SNAPSHOTTER/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
kubectl apply -f "$SNAPSHOTTER/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
# Run these two lines only if the cluster has no compatible snapshot controller.
kubectl apply -f "$SNAPSHOTTER/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
kubectl apply -f "$SNAPSHOTTER/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
kubectl -n kube-system rollout status deployment/snapshot-controller
```

The static deployment previously installed snapshot CRDs from
`deploy/kubernetes/crd-upcloud-csi.yaml` and a shared controller from
`setup-upcloud-csi.yaml`. Its `csi-upcloud-snapshot-controller` is a
cluster-wide upstream snapshot controller, **not** the UpCloud driver. The
driver-specific `csi-snapshotter` sidecar runs in `csi-upcloud-controller` and
talks to the driver over its CSI socket.

## Install

For token authentication, create the Secret instead with
`--from-literal=token="$UPCLOUD_TOKEN"`. If a non-empty `token` and username/password are both present, the driver uses the token, not username/password. Remove the token key to use username/password. The keys are optional in the pod specification so either Secret shape works; if no valid credentials are provided, the controller driver reports an error and exits. Updating the Secret requires restarting the controller pod to refresh environment variables.

From the repository root:

```bash
helm install upcloud-csi charts/upcloud-csi-driver --namespace kube-system \
  --set zone=fi-hel1 \
  --set credentials.secretName=upcloud-csi-credentials
```

The chart does not create the credentials Secret. The controller requires that Secret; the node DaemonSet runs without API credentials.

Object names are fixed (`csi-upcloud-controller`, `csi-upcloud-node`, and the RBAC and storage class names from the static manifests). Install one release per cluster.

By default, the chart creates `upcloud-csi-snapshotclass`, so the snapshot
CRDs must exist before Helm installs the chart. The sidecar also needs those
APIs and a shared snapshot controller to perform snapshot operations. Check
the workloads and class after installation:

```bash
kubectl -n kube-system rollout status deployment/csi-upcloud-controller
kubectl -n kube-system rollout status daemonset/csi-upcloud-node
kubectl get volumesnapshotclass upcloud-csi-snapshotclass -o yaml
```

When snapshots are not required, set `--set volumeSnapshotClass.enabled=false`
to omit only the VolumeSnapshotClass. This does not disable the CSI snapshotter
sidecar; without the CRDs, it may log missing-API errors and cannot provide
snapshot functionality.

## Configuration

| Value | Default | Purpose |
| --- | --- | --- |
| `zone` | `""` | Required UpCloud zone for both controller and node pods; no zone is selected by default |
| `image.repository` | `ghcr.io/upcloudltd/upcloud-csi` | Driver image repository |
| `image.tag` | `v1.5.1` | Pinned driver image tag |
| `sidecars.provisioner.repository` / `tag` | `registry.k8s.io/sig-storage/csi-provisioner` / `v3.1.0` | external-provisioner |
| `sidecars.attacher.repository` / `tag` | `registry.k8s.io/sig-storage/csi-attacher` / `v3.4.0` | external-attacher |
| `sidecars.resizer.repository` / `tag` | `registry.k8s.io/sig-storage/csi-resizer` / `v1.4.0` | external-resizer |
| `sidecars.snapshotter.repository` / `tag` | `registry.k8s.io/sig-storage/csi-snapshotter` / `v4.2.1` | external-snapshotter |
| `sidecars.snapshotController.repository` / `tag` | `registry.k8s.io/sig-storage/snapshot-controller` / `v4.2.1` | snapshot-controller |
| `sidecars.nodeDriverRegistrar.repository` / `tag` | `registry.k8s.io/sig-storage/csi-node-driver-registrar` / `v2.5.0` | node-driver-registrar |
| `controller.replicas` | `1` | Controller Deployment replicas; keep at 1 |
| `controller.tolerations` | `[]` | Tolerations for the controller pod |
| `snapshotController.enabled` | `false` | Opt into the historical shared snapshot controller only if the cluster has none |
| `snapshotController.replicas` | `2` | Optional shared snapshot-controller Deployment replicas |
| `credentials.secretName` | `upcloud-csi-credentials` | Existing Secret name |
| `resources.controller.*` | `{}` | Requests and limits for provisioner, attacher, resizer, snapshotter, and plugin |
| `resources.snapshotController` | `{}` | Requests and limits for snapshot-controller |
| `resources.node.registrar` / `resources.node.plugin` | `{}` | Requests and limits for the node pods |
| `storageClasses.enabled` | `true` | Create the four UpCloud StorageClasses |
| `storageClasses.defaultClass` | `upcloud-block-storage-maxiops` | Chart-managed StorageClass to mark as default; set to `""` for none |
| `volumeSnapshotClass.enabled` | `true` | Create `upcloud-csi-snapshotclass` |

Empty resource values omit the `resources` block, which is what the static manifests do.

Keep `controller.replicas` at 1: the controller sidecars do not use leader election (the csi-snapshotter explicitly disables it), and `Recreate` does not prevent concurrent replicas if the replica count is greater than 1. The optional shared snapshot-controller Deployment enables leader election and defaults to 2 replicas. Use `snapshotController.enabled=true` only when intentionally letting this release manage the cluster-wide controller instead of installing one separately. It still requires the administrator to install the CRDs.
Set the required `zone` to the UpCloud zone where the driver runs. Both node and controller modes require it; the same zone is passed to every node pod, so this chart assumes all nodes use that zone.

To leave another StorageClass as default, set `--set storageClasses.defaultClass=""` or set `storageClasses.defaultClass: ""` in a values file. All chart-managed classes are then annotated as non-default. To select the encrypted maxiops class instead, set `--set storageClasses.defaultClass=upcloud-block-storage-maxiops-encrypted`. Other valid choices are `upcloud-block-storage-maxiops`, `upcloud-block-storage-hdd`, and `upcloud-block-storage-standard`. The chart does not change StorageClasses it does not manage; check for existing defaults before selecting one. Encrypted snapshots are not supported as volume sources.

Example resource override:

```yaml
resources:
  node:
    plugin:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        memory: 256Mi
```

Example controller toleration:

```yaml
controller:
  tolerations:
    - key: node-role.kubernetes.io/control-plane
      operator: Exists
      effect: NoSchedule
```

```bash
helm install upcloud-csi charts/upcloud-csi-driver \
  --namespace kube-system \
  --set zone=fi-hel1 \
  --values my-values.yaml
```

Override `image.repository` and `image.tag`, or any `sidecars.*.repository` and `sidecars.*.tag`, to use different images. The defaults use driver release v1.5.1 and the existing static-manifest sidecar versions; the static driver manifest still uses `latest`.

## Upgrade

```bash
helm upgrade upcloud-csi charts/upcloud-csi-driver --namespace kube-system \
  --set zone=fi-hel1
```

Changing the driver or sidecar image updates the controller Deployment and node DaemonSet pod templates and rolls those pods. The controller Deployment uses `Recreate`: old controller pods are terminated before replacements start, so controller operations can be briefly unavailable. Existing mounted volumes continue to be served by node pods.

Snapshot CRDs and an externally managed shared controller are not changed by
Helm upgrades. An upgrade from a chart release that managed the shared controller
will remove that Deployment unless `snapshotController.enabled=true` is set;
ensure another compatible controller is running before switching ownership.
CRDs installed by an older chart remain in the cluster and must be maintained
separately.

## Uninstall

```bash
helm uninstall upcloud-csi --namespace kube-system
```

Helm removes the workloads, RBAC, CSIDriver, StorageClasses, and VolumeSnapshotClass created by the release.

Helm leaves these objects in place:

- the credentials Secret
- PersistentVolumes (the chart StorageClasses use `reclaimPolicy: Retain`)
- VolumeSnapshot objects that already exist
- externally managed snapshot CRDs and snapshot controller

Removing a StorageClass leaves existing volumes in place. PersistentVolumeClaims that name that class need the StorageClass to exist again before a new claim can use it.

## Migrating an existing installation

The chart manages the driver objects from `rbac-upcloud-csi.yaml` and `setup-upcloud-csi.yaml`. A fresh `helm install` fails while those objects already exist, because they have no Helm ownership metadata.

Adopt them with Helm 3.17 or newer:

```bash
helm upgrade --install upcloud-csi charts/upcloud-csi-driver \
  --namespace kube-system \
  --set zone=fi-hel1 \
  --set credentials.secretName=upcloud \
  --set snapshotController.enabled=true \
  --take-ownership
```

The release namespace has to be `kube-system` when the current objects live there.
This example adopts the historical shared controller; use it only if that is
the cluster's intended snapshot controller. Otherwise remove or migrate the
historical controller separately and omit `snapshotController.enabled=true`.
The static manifests reference Secret `upcloud`, while the chart defaults to `upcloud-csi-credentials`. Set `credentials.secretName=upcloud` when retaining the existing Secret. The chart also changes the default driver image from `latest` to `v1.5.1` and passes the configured zone to node pods; check the image and zone before adoption.

On older Helm, annotate and label each existing object before `helm upgrade --install`:

```bash
kubectl annotate <kind> <name> \
  meta.helm.sh/release-name=upcloud-csi \
  meta.helm.sh/release-namespace=kube-system \
  --overwrite
kubectl label <kind> <name> app.kubernetes.io/managed-by=Helm --overwrite
```

Do that for namespaced objects in `kube-system` and for cluster-scoped objects (CSIDriver, StorageClass, VolumeSnapshotClass, ClusterRole, ClusterRoleBinding). Leave the credentials Secret unmanaged.

Adoption adds chart labels to pod templates and rolls the controller and node pods. Selectors stay `app: csi-upcloud-controller` and `app: csi-upcloud-node`.

Clusters that already applied `crd-upcloud-csi.yaml` keep those CRDs. Helm does not replace them.

The optional snapshot validation webhook stays outside this chart. Continue to apply `deploy/kubernetes/snapshot-webhook-upcloud-csi.yaml` when you want that webhook. It expects Secret `snapshot-validation-secret` and the ServiceAccount this chart creates.

StorageClasses and the node-driver-registrar ClusterRole are cluster-scoped. The static manifests set `metadata.namespace: kube-system` on those objects; `kubectl apply` drops that field. The chart omits it so Helm can create them.

The optional shared controller retains the static Deployment and ServiceAccount
names. Its leader-election RoleBinding is corrected to grant permissions to
the ServiceAccount used by that Deployment.

## Validate

`charts/upcloud-csi-driver/scripts/verify.py` runs `helm lint`, renders the chart with a zone and other default or custom values, and checks it against the static manifests with documented chart-specific differences. It needs Helm and PyYAML.

```bash
pip install pyyaml
python3 charts/upcloud-csi-driver/scripts/verify.py
```
