# Kubernetes

UpCloud CSI driver deployment is bundled with [sidecar containers](#sidecars) and optional [snapshot validation webhook](#deploy-snapshot-validation-webhook) service.  
Version specific deployment manifests `upcloud-csi-crd.yaml` and `upcloud-csi-setup.yaml` can be found under [release assets](https://github.com/UpCloudLtd/upcloud-csi/releases/latest/).

## Deployment

### Requirements

* Kubernetes v1.16+
* UpCloud account

### Create a Kubernetes secret

Execute the following commands to add UpCloud credentials as Kubernetes secret:  
<sub>_Replace `$UPCLOUD_PASSWORD` and `$UPCLOUD_USERNAME` with your UpCloud API credentials if not defined using environment variables._</sub>
```shell
$ kubectl -n kube-system create secret generic upcloud --from-literal=password=$UPCLOUD_PASSWORD --from-literal=username=$UPCLOUD_USERNAME
```

After the message, that secret was created, you can run this command to check the existence of `upcloud` secret
in `kube-system` namespace:

```shell
$ kubectl -n kube-system get secret upcloud
NAME             TYPE                                  DATA      AGE
upcloud          Opaque                                2         18h
```

### Deploy CSI Driver

Deploy custom resources definitions required by CSI driver:
```shell
$ kubectl apply -f https://github.com/UpCloudLtd/upcloud-csi/releases/latest/download/upcloud-csi-crd.yaml
```

Deploy the CSI driver with the related Kubernetes volume attachment, driver registration, and provisioning sidecars:
```shell
$ kubectl apply -f https://github.com/UpCloudLtd/upcloud-csi/releases/latest/download/upcloud-csi-setup.yaml
```

#### Deploy snapshot validation webhook
The snapshot validating webhook is **optional** service that provides tightened validation on snapshot objects. 
Service is optional but recommended if volume snapshots are used in cluster.  
Validation service requires proper CA certificate, x509 certificate and matching private key for secure communication.  
More information can be found from official snapshot [webhook example](https://github.com/kubernetes-csi/external-snapshotter/tree/master/deploy/kubernetes/webhook-example) along with example how to deploy certificates.

Manifest `snapshot-webhook-upcloud-csi.yaml` can be used to deploy webhook service.  
Manifest assumes that secret named `snapshot-validation-secret` exists and is populated with valid x509 certificate `cert.pem` (CA cert, if any, concatenated after server cert) and matching private key `key.pem`.
If custom CA is used (e.g. when using self-signed certificate) `caBundle` field needs to be set with CA data as value.

```shell
$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/main/deploy/kubernetes/snapshot-webhook-upcloud-csi.yaml
```


### Choose storage disk type

It's possible to select an option of disk type between `HDD` and `MaxIOPS`.
For setting desired type you can set a `storageClassName` field in `PVC` to:
* `upcloud-block-storage-maxiops`
* `upcloud-block-storage-hdd`

If `storageClassName` field is not set, the default provisioned option will be `upcloud-block-storage-maxiops`.

### Example Usage

In `example` directory you may find 2 manifests for deploying a pod and persistent volume claim to test CSI Driver
operations

```shell
$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/main/example/test-pod.yaml
$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/main/example/test-pvc.yaml
```

Check if pod is deployed with Running status and already using the PVC:

```shell
$ kubectl get pvc/csi-pvc pods/csi-app
$ kubectl describe pvc/csi-pvc pods/csi-app | less
```

To check the persistence feature - you may create the sample file in Pod, later, delete the pod and re-create it from yaml manifest and notice that the file is still in mounted directory 

```shell
$ kubectl exec -it csi-app -- /bin/sh -c "touch /data/persistent-file.txt"
total 24K
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

$ kubectl delete pods/csi-app
pod "csi-app" deleted

$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/main/example/test-pod.yaml
pod/csi-app created

$ kubectl exec -it csi-app -- /bin/sh -c "ls -l /data"
total 20
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

```

## Sidecars

Kubernetes CSI sidecar containers are a set of standard containers which contain common logic to watch the Kubernetes API, trigger appropriate operations against the UpCloud CSI driver container, and update the Kubernetes API as appropriate.


### Controller
#### [external-provisioner](https://github.com/kubernetes-csi/external-provisioner)
Watches the Kubernetes API server for `PersistentVolumeClaim` objects and triggers `CreateVolume` and `DeleteVolume` operations against the driver.  
Provides the ability to request a volume be pre-populated from a [data source](https://kubernetes-csi.github.io/docs/volume-datasources.html) during provisioning. 
- Image: k8s.gcr.io/sig-storage/csi-provisioner
- Controller capability: `CREATE_DELETE_VOLUME`

#### [external-attacher](https://github.com/kubernetes-csi/external-attacher)
Watches the Kubernetes API server for `VolumeAttachment` objects and triggers `ControllerPublishVolume` and `ControllerUnpublishVolume` operations against the driver. 
- Image: k8s.gcr.io/sig-storage/csi-attacher
- Controller capability: `PUBLISH_UNPUBLISH_VOLUME`

### [external-snapshotter](https://github.com/kubernetes-csi/external-snapshotter) <sub>Beta/GA</sub>
Watches the Kubernetes API server for `VolumeSnapshot` and `VolumeSnapshotContent` CRD objects and triggers `CreateSnapshot`, `DeleteSnapshot` and `ListSnapshots` operations against the driver. 
- Image: k8s.gcr.io/sig-storage/csi-snapshotter
- Controller capability: `CREATE_DELETE_SNAPSHOT`, `LIST_SNAPSHOTS`

### [external-resizer](https://github.com/kubernetes-csi/external-resizer)
Watches the Kubernetes API server for `PersistentVolumeClaim` object edits and triggers `ControllerExpandVolume` operation against the driver if volume size is increased.
- Image: k8s.gcr.io/sig-storage/csi-resizer
- Plugin capability: `VolumeExpansion_OFFLINE`

### Node
### [node-driver-registrar](https://github.com/kubernetes-csi/node-driver-registrar)
Fetches driver information using `NodeGetInfo` from the driver and registers it with the kubelet on that node using unix domain socket.  
Kubelet triggers `NodeGetInfo`, `NodeStageVolume`, and `NodePublishVolume` operations against the driver. 
- Image: k8s.gcr.io/sig-storage/csi-node-driver-registrar
