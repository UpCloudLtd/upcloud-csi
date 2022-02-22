# UpCloud CSI Driver

## Overview

UpCloud [CSI](https://github.com/container-storage-interface/spec) Driver provides a basis for using the UpCloud Storage
service in [CO](https://www.vmware.com/topics/glossary/content/container-orchestration.html) systems, such as
Kubernetes, to obtain stateful application deployment with ease.

Additional info about the CSI can be found
in [Kubernetes CSI Developer Documentation](https://kubernetes-csi.github.io/docs/example.html)
and [Kubernetes Blog](https://github.com/container-storage-interface/spec/).

## Disclaimer

Before reaching the **v1.0.0** version, UpCloud CSI Driver is **NOT** recommended for production environment usage.

## Deployment

### Releases

| Version | Date       |
|---------|------------|
| alpha   | 22-02-2022 |
| beta    | TBD        |
| 1.0.0   | TBD        |


### Requirements

* Kubernetes v1.16+
* UpCloud account

### Create a Kubernetes secret

Execute the following commands to add UpCloud credentials as Kubernetes secret

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
in `kube-system` namespace

```sh
$ kubectl -n kube-system get secret upcloud
NAME                  TYPE                                  DATA      AGE
upcloud          Opaque                                2         18h
```

### Deploy CSI Driver

The following command will deploy the CSI driver with the related Kubernetes volume attachment, driver registration, and
provisioning sidecars:

```sh
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/alpha/deploy/kubernetes/alpha/driver.yaml
```

### Example Usage

In `example` directory you may find 2 manifests for deploying a pod and persistent volume claim to test CSI Driver
operations

```sh
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/alpha/example/test-pod.yaml
kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/alpha/example/test-pvc.yaml
```

Check if pod is deployed with Running status and already using the PVC:

```sh
kubectl get pvc/csi-pvc pods/csi-app
kubectl describe pvc/csi-pvc pods/csi-app | less
```

Now, let's add some data into the PVC, delete the POD, and recreate the pod. Our data will remain intact.

```sh
$ kubectl exec -it csi-app -- /bin/sh -c "touch /data/persistent-file.txt"
total 24K
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

$ kubectl delete pods/csi-app
pod "csi-app" deleted

$ kubectl apply -f https://raw.githubusercontent.com/UpCloudLtd/upcloud-csi/alpha/example/test-pod.yaml
pod/csi-app created

$ kubectl exec -it csi-app -- /bin/sh -c "ls -l /data"
total 20
-rw-r--r--    1 root     root           0 Feb 22 12:29 persistent-file.txt
drwx------    2 root     root       16.0K Feb 22 12:25 lost+found

```

## Contribution

Feel free to open PRs and issues, as the development of CSI driver is in progress.
