# Developing the CSI driver

CSI driver's primary goal is to conform to [Container Storage Interface (CSI)](https://github.com/container-storage-interface/spec/blob/6bdbaa0472f5a1dc0e0e1f3738c65b4cac951d1f/spec.md) specification by implementing required gRPC endpoints. Unsupported endpoints should return an `CALL_NOT_IMPLEMENTED` error.  
Depending on [CO](https://www.vmware.com/topics/glossary/content/container-orchestration.html), endpoints are called directly or by [sidecar containers](deploy/kubernetes/README.md#sidecars).

## Requirements
- [Go](https://golang.org/doc/install) > 1.17

Get the source code:

```shell
$ git clone git@github.com:UpCloudLtd/upcloud-csi.git
$ cd upcloud-csi
```

To compile the plugin, run `make build-plugin`. This will build the driver plugin binary `cmd/upcloud-csi-plugin/upcloud-csi-plugin` .

```shell
$ make build-plugin
```

## Project layout
### Applications
Project applications can be found under `cmd` directory:
- `upcloud-csi-plugin` is monolith CSI driver that can be run as controller or node driver (or both).
- `upcloud-csi-manifest` is for rendering version specific Kubernetes deployment manifests

### Driver
Required interfaces are implemented by `Driver` type in `driver` package. For clarity, interface implementations are separated based on their role to files `controller.go`, `node.go` and `identity.go`.  
Driver's gRPC server exposes endpoints described in following interfaces:
- [csi.IdentityServer](https://pkg.go.dev/github.com/container-storage-interface/spec@v1.6.0/lib/go/csi#IdentityServer)
- [csi.ControllerServer](https://pkg.go.dev/github.com/container-storage-interface/spec@v1.6.0/lib/go/csi#ControllerServer)
- [csi.NodeServer](https://pkg.go.dev/github.com/container-storage-interface/spec@v1.6.0/lib/go/csi#NodeServer)

It's good to keep in mind that althought same `Driver` type implementes all the interfaces, node (`csi.NodeServer`) and controller (`csi.ControllerServer`) endpoints are normally executed on different hosts.  

## Testing
Run tests using `make`
```shell
$ make test
```

## Logging
Driver uses structured logging which level can be set using `--log-level` flag. Only errors are logged by default. OS level commands are logged using `DEBUG` level which also logs gRPC request and response objects. Debug level is only suitable for debugging purposes.  
Logging keys are defined in [driver/log.go](driver/log.go) to keep keys consistent across driver.  
Correlation ID (`correlation_id`) is attached to log messages using request interceptor (aka middleware) so that driver operations can be tracked across controller and node.

## Tooling
CSI driver's controller functionality can be tested locally but node functions requires that driver is run in UpCloud VM so that driver can see attached disks. 

Following example commands expects that driver is running and using endpoint `/tmp/csi.sock` e.g:
```shell
$ upcloud-csi-plugin --username=$UPCLOUD_USERNAME --password=$UPCLOUD_PASSWORD --nodehost=$HOSTNAME --endpoint=unix:///tmp/csi.sock --log-level=debug
```

### Sanity Test Command Line Program
[Sanity Test](https://github.com/kubernetes-csi/csi-test/tree/master/cmd/csi-sanity) is the command line program that tests a CSI driver using the [sanity](https://github.com/kubernetes-csi/csi-test/tree/master/pkg/sanity) package test suite.
```shell
$ csi-sanity --csi.endpoint=/tmp/csi.sock --ginkgo.fail-fast -csi.testnodevolumeattachlimit
```

### Container Storage Client
The [Container Storage Client (csc)](https://github.com/rexray/gocsi/tree/master/csc) is a command line interface (CLI) tool that provides analogues for all of the CSI RPCs.
Print command help
```shell
$ csc -e unix:///tmp/csi.sock help
```
Get controller capabilities
```shell
$ csc -e unix:///tmp/csi.sock controller get-capabilities
&{type:CREATE_DELETE_VOLUME }
&{type:PUBLISH_UNPUBLISH_VOLUME }
&{type:LIST_VOLUMES }
&{type:CREATE_DELETE_SNAPSHOT }
&{type:LIST_SNAPSHOTS }
&{type:EXPAND_VOLUME }
&{type:CLONE_VOLUME }
```

