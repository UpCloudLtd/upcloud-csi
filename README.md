# UpCloud CSI Driver ![GitHub Actions status](https://github.com/UpCloudLtd/upcloud-csi/actions/workflows/deploy.yml/badge.svg)

## Overview

UpCloud [CSI](https://github.com/container-storage-interface/spec) Driver provides a basis for using the UpCloud Storage
service in [CO](https://www.vmware.com/topics/glossary/content/container-orchestration.html) systems, such as
Kubernetes, to obtain stateful application deployment with ease.

Additional info about the CSI can be found
in [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/)
and [Kubernetes Blog](https://kubernetes.io/blog/2019/01/15/container-storage-interface-ga/).

## Disclaimer

Before reaching the **v1.0.0** version, UpCloud CSI Driver is **NOT** recommended for production environment usage.

## Deployment

### Requirements

* Kubernetes v1.16+
* UpCloud account

### Create a Kubernetes secret

Execute the following commands to add UpCloud credentials as Kubernetes secret:

```bash
export UPCLOUD_USERNAME=your-username && export UPCLOUD_PASSWORD=your-password
cat <<EOF | kubectl apply -f -
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: upcloud
  namespace: kube-system
stringData:
  username: "$UPCLOUD_USERNAME"
  password: "$UPCLOUD_PASSWORD"
EOF
```

After the message, that secret was created, you can run this command to check the existence of `upcloud` secret
in `kube-system` namespace:

```sh
$ kubectl -n kube-system get secret upcloud
NAME                  TYPE                                  DATA      AGE
upcloud          Opaque                                2         18h
```

### Deploy CSI Driver

The following commands will deploy the CSI driver with the related Kubernetes custom resources, volume attachment, driver registration, and
provisioning sidecars:

```sh
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/deploy/kubernetes/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/deploy/kubernetes/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/deploy/kubernetes/crd/snapshot.storage.k8s.io_volumesnapshots.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/deploy/kubernetes/rbac-upcloud-csi.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/deploy/kubernetes/setup-upcloud-csi.yaml
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

```sh
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pod.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pvc.yaml
```

Check if pod is deployed with Running status and already using the PVC:

```sh
kubectl get pvc/csi-pvc pods/csi-app
kubectl describe pvc/csi-pvc pods/csi-app | less
```

To check the persistence feature - you may create the sample file in Pod, later, delete the pod and re-create it from yaml manifest and notice that the file is still in mounted directory 

```sh
$ kubectl exec -it csi-app -- /bin/sh -c "touch /data/persistent-file.txt"
total 24K
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

$ kubectl delete pods/csi-app
pod "csi-app" deleted

$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pod.yaml
pod/csi-app created

$ kubectl exec -it csi-app -- /bin/sh -c "ls -l /data"
total 20
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

```

## Contribution

Feel free to open PRs and issues, as the development of CSI driver is in progress.
