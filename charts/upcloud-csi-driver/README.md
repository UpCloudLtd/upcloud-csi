# UpCloud CSI Driver

Helm chart for the Kubernetes deployment in [`deploy/kubernetes`](../../deploy/kubernetes).
Resource names, images, RBAC, and storage classes match those manifests. The chart runs separate node and controller driver modes.

The chart installs:

- CSIDriver `storage.csi.upcloud.com`
- Snapshot CRDs from `crds/` (external-snapshotter v4.2)
- Controller Deployment, snapshot-controller Deployment, and node DaemonSet
- Service accounts and RBAC
- StorageClasses and a VolumeSnapshotClass, unless disabled

Install it into the namespace that contains the UpCloud credentials Secret. The static manifests use `kube-system`.

UpCloud Managed Kubernetes (UKS) already includes this driver. Skip this chart on UKS clusters.

## Prerequisites

- Kubernetes 1.16 or newer
- Helm 3 or newer
- A pre-existing Kubernetes Secret with keys `username` and `password`. The recommended name is `upcloud-csi-credentials`.

```bash
kubectl -n kube-system create secret generic upcloud-csi-credentials \
  --from-literal=username="$UPCLOUD_USERNAME" \
  --from-literal=password="$UPCLOUD_PASSWORD"
```

## Install

From the repository root:

```bash
helm install upcloud-csi charts/upcloud-csi-driver --namespace kube-system
```

The chart does not create the credentials Secret. The controller requires that Secret; the node DaemonSet runs without API credentials.

Object names are fixed (`csi-upcloud-controller`, `csi-upcloud-node`, and the RBAC and storage class names from the static manifests). Install one release per cluster.

## Configuration

| Value | Default | Purpose |
| --- | --- | --- |
| `image.repository` | `ghcr.io/upcloudltd/upcloud-csi` | Driver image repository |
| `image.tag` | `latest` | Driver image tag. The static manifest uses `latest` with `imagePullPolicy: Always` |
| `sidecars.provisioner.repository` / `tag` | `k8s.gcr.io/sig-storage/csi-provisioner` / `v3.1.0` | external-provisioner |
| `sidecars.attacher.repository` / `tag` | `k8s.gcr.io/sig-storage/csi-attacher` / `v3.4.0` | external-attacher |
| `sidecars.resizer.repository` / `tag` | `k8s.gcr.io/sig-storage/csi-resizer` / `v1.4.0` | external-resizer |
| `sidecars.snapshotter.repository` / `tag` | `k8s.gcr.io/sig-storage/csi-snapshotter` / `v4.2.1` | external-snapshotter |
| `sidecars.snapshotController.repository` / `tag` | `k8s.gcr.io/sig-storage/snapshot-controller` / `v4.2.1` | snapshot-controller |
| `sidecars.nodeDriverRegistrar.repository` / `tag` | `k8s.gcr.io/sig-storage/csi-node-driver-registrar` / `v2.5.0` | node-driver-registrar |
| `controller.replicas` | `1` | `csi-upcloud-controller` Deployment replicas |
| `controller.zone` | `""` | UpCloud zone used by the controller. Set this when deploying the controller in controller mode |
| `controller.tolerations` | `[]` | Tolerations for the controller pod |
| `snapshotController.replicas` | `2` | snapshot-controller Deployment replicas |
| `credentials.secretName` | `upcloud` | Existing Secret name |
| `resources.controller.*` | `{}` | Requests and limits for provisioner, attacher, resizer, snapshotter, and plugin |
| `resources.snapshotController` | `{}` | Requests and limits for snapshot-controller |
| `resources.node.registrar` / `resources.node.plugin` | `{}` | Requests and limits for the node pods |
| `storageClasses.enabled` | `true` | Create the three UpCloud StorageClasses |
| `volumeSnapshotClass.enabled` | `true` | Create `upcloud-csi-snapshotclass` |

Empty resource values omit the `resources` block, which is what the static manifests do.

`controller.replicas` defaults to 1. The csi-snapshotter container is started with `--leader-election=false`, so keep a single controller replica unless you also change that sidecar. The snapshot-controller Deployment runs with `--leader-election=true` and defaults to 2 replicas.
Set `controller.zone` to the UpCloud zone where the driver runs; the controller mode requires a zone.

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
  --values my-values.yaml
```

Pin `image.tag` for a reproducible install. The default `latest` tag matches the static manifest.

## Upgrade

```bash
helm upgrade upcloud-csi charts/upcloud-csi-driver --namespace kube-system
```

Changing the driver or sidecar image updates the controller Deployment and node DaemonSet pod templates and rolls those pods.

Helm installs CRDs from `crds/` when they are absent. `helm upgrade` does not change CRDs that already exist, and `helm rollback` does not revert them. To pick up a newer snapshot CRD, apply `deploy/kubernetes/crd-upcloud-csi.yaml` yourself.

## Uninstall

```bash
helm uninstall upcloud-csi --namespace kube-system
```

Helm removes the workloads, RBAC, CSIDriver, StorageClasses, and VolumeSnapshotClass created by the release.

Helm leaves these objects in place:

- the credentials Secret
- PersistentVolumes (the chart StorageClasses use `reclaimPolicy: Retain`)
- VolumeSnapshot objects that already exist
- snapshot CRDs

Removing a StorageClass leaves existing volumes in place. PersistentVolumeClaims that name that class need the StorageClass to exist again before a new claim can use it.

## Migrating an existing installation

The chart manages the same objects as `rbac-upcloud-csi.yaml` and `setup-upcloud-csi.yaml`. A fresh `helm install` fails while those objects already exist, because they have no Helm ownership metadata.

Adopt them with Helm 3.17 or newer:

```bash
helm upgrade --install upcloud-csi charts/upcloud-csi-driver \
  --namespace kube-system \
  --take-ownership
```

The release namespace has to be `kube-system` when the current objects live there.

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

Two details are copied from the static manifests:

- The snapshot-controller leader-election RoleBinding subject is ServiceAccount `snapshot-controller`. The Deployment uses ServiceAccount `csi-upcloud-controller-sa`.
- The controller runs as a Deployment.

## Validate

`charts/upcloud-csi-driver/scripts/verify.py` runs `helm lint`, renders the chart, and checks the result against the static manifests. It needs Helm and PyYAML.

```bash
pip install pyyaml
python3 charts/upcloud-csi-driver/scripts/verify.py
```
