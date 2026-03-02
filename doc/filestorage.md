# File Storage driver (aka NFS + UKS)

Our CSI driver is actually multiple drivers in a trenchcoat; the actual driver
is selected on startup based on command line flags, defaulting to the block
`Storage` driver. This means that you'll need a second instance to use the File
Storage driver, even though they are in the same binary.

This is for technical reasons, like dealing with different capabilities of the
different drivers and helping the Container Orchestrator to identify which
driver to use for a volume.

Example manifests to deploy the File Storage driver are provided in `deploy/`.

## Installation procedure

* create a File Storage for the CSI driver to use
* create a subaccount the CSI driver can use
  - on Hub go to People -> Create subaccount and fill the form
  - after finalizing, go to the details page of that newly created subaccount and click "Go to permissions"
  - enable those permissions:
    + API connections
    + Servers
    + Private Networks (you can select specific networks here if you want; the ones used for your UKS clusters)
    + File Storage (you can select specific File Storages here; the ones you want the CSI driver to use)
      * TODO: this is not available yet!

### UKS-specific

* create a secret in your cluster(s) with the credentials
  - `kubectl create secret generic -n kube-system upcloud-csi-filestorage --from-literal username=USERNAME_OF_SUBACCOUNT --from-literal password=PASSWORD_OF_SUBACCOUNT`
* apply deployment manifest
  - `kubectl apply -f deploy/kubernetes/setup-upcloud-csi-filestorage.yaml`
* create a `StorageClass`
  - ```
    kubectl apply -f - <<EOF
    kind: StorageClass
    apiVersion: storage.k8s.io/v1
    metadata:
      name: file-storage
    parameters:
      fileStorage: FILE_STORAGE_UUID
    provisioner: filestorage.csi.upcloud.com
    reclaimPolicy: Delete # you may want to change this
    EOF
    ```
* you can now create `PersistentVolumeClaims` using the `StorageClass` `file-storage`!


## Features and limitations

The File Storage driver will
* attach a suitable private network to the File Storage upon mounting a Volume
  on a given Node
  - but currently only a single network can be attached to a File Storage
    instance
  - the driver will never detach that network, since it doesn't have any
    information about currently mounted Volumes when unmounting any single
    Volume
* create and delete Shares on the File Storage given in the parameters (in UKS:
  via `StorageClass`)
* create and delete ACLs on Shares upon mounting/unmounting a Share from a
  given Node

You can also have the File Storage driver mount and unmount manually created
Shares; the Volume ID format is `$fileStorageUUID/$shareName` (so e.g.
`1732c6ec-2a71-4bdf-9078-63a414743cd4/my-manually-created-share`). Due to
limitations of the CSI protocol, that full Volume ID can only be up to 128
bytes long (= characters; when sticking to ASCII characters), limiting the max
length of the share name.

Due to the File Storage service not having Quota support, Volumes are not
limited in size and the requested amount of storage available is completely
ignored. This also means that you will have to manage the size of File Storage
instance yourself. At any given time, the free space on a File Storage instance
is available to all the volumes - they fully share the capacity of the File
Storage.
