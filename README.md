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

#### Deploy snapshot validation webhook (optional)
The snapshot validating webhook is service that provides tightened validation on snapshot objects. 
Service is optional but recommended if volume snapshots are used in cluster.  
Validation service requires proper CA certificate, x509 certificate and matching private key for secure communication.  
More information can be found from official snapshot [webhook example](https://github.com/kubernetes-csi/external-snapshotter/tree/master/deploy/kubernetes/webhook-example) along with example how to deploy certificates.

Manifest `snapshot-webhook-upcloud-csi.yaml` can be used to deploy webhook service.  
Manifest assumes that secret named `snapshot-validation-secret` exists and is populated with valid x509 certificate `cert.pem` (CA cert, if any, concatenated after server cert) and matching private key `key.pem`.
If custom CA is used (e.g. when using self-signed certificate) `caBundle` field needs to be set with CA data as value.

```sh
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/main/deploy/kubernetes/snapshot-webhook-upcloud-csi.yaml
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
$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pod.yaml
$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pvc.yaml
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

$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/example/test-pod.yaml
pod/csi-app created

$ kubectl exec -it csi-app -- /bin/sh -c "ls -l /data"
total 20
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

```

## Contribution

Feel free to open PRs and issues, as the development of CSI driver is in progress.
