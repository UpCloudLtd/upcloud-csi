# HashiCorp Nomad
UpCloud CSI plugin can be used to run stateful workloads inside your [HashiCorp Nomad](https://www.nomadproject.io/) cluster.  

CSI documentation from HashiCorp:  
- [Stateful Workloads with Container Storage Interface](https://learn.hashicorp.com/tutorials/nomad/stateful-workloads-csi-volumes?in=nomad/stateful-workloads)
- [csi_plugin](https://developer.hashicorp.com/nomad/docs/job-specification/csi_plugin) Stanza


## Deployment

UpCloud CSI plugin supports all plugin [types](https://developer.hashicorp.com/nomad/docs/job-specification/csi_plugin#type): `node`, `controller` and `monolith`. CSI Plugins running as `node` or `monolith` type require root privileges (or CAP_SYS_ADMIN on Linux) to mount volumes on the host, see more information from Nomad [documentation](https://learn.hashicorp.com/tutorials/nomad/stateful-workloads-csi-volumes?in=nomad/stateful-workloads).  

### Monolith example
Download [upcloud-csi.hcl](upcloud-csi.hcl) and run job:  
<sub>_Replace `$UPCLOUD_PASSWORD` and `$UPCLOUD_USERNAME` with your UpCloud API credentials if not defined using environment variables._</sub>
```shell
$ sudo nomad run \
    -var="upcloud_username=$UPCLOUD_USERNAME" \
    -var="upcloud_password=$UPCLOUD_PASSWORD" \
    -var="upcloud_zone=$UPCLOUD_ZONE" \
    upcloud-csi.hcl
```
Check UpCloud CSI plugin status:
```shell
$ sudo nomad plugin status csi-upcloud 
ID                   = csi-upcloud
Provider             = storage.csi.upcloud.com
Version              = <none>
Controllers Healthy  = 1
Controllers Expected = 1
Nodes Healthy        = 1
Nodes Expected       = 1

Allocations
ID        Node ID   Task Group  Version  Desired  Status   Created     Modified
f7a36e62  656e4c06  nodes       1        run      running  18m25s ago  18m14s ago
```
Output might vary depending on your Nomad setup but there should be at least one healthy node and controller.

## Persistent volume example
Download [example-volume.hcl](example-volume.hcl) and create new volume
```shell
$ sudo nomad volume create example-volume.hcl
```
Check volume status:
```shell
$ sudo nomad volume status my-volume 
ID                   = my-volume
Name                 = My persistent volume
External ID          = 01436977-2ad8-43e9-8658-fd26cc463d2a
Plugin ID            = csi-upcloud
Provider             = storage.csi.upcloud.com
Version              = <none>
Schedulable          = true
Controllers Healthy  = 1
Controllers Expected = 1
Nodes Healthy        = 1
Nodes Expected       = 1
Access Mode          = <none>
Attachment Mode      = <none>
Mount Options        = <none>
Namespace            = default

Topologies
Topology  Segments
01        region=pl-waw1

Allocations
No allocations placed
```

See [Stateful Workloads with Container Storage Interface](https://learn.hashicorp.com/tutorials/nomad/stateful-workloads-csi-volumes?in=nomad/stateful-workloads) for more information about how to use volume.