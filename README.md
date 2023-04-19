# UpCloud CSI Driver  
![GitHub Actions status](https://github.com/UpCloudLtd/upcloud-csi/actions/workflows/deploy.yaml/badge.svg)

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
### Kubernetes 
Kubernetes deployment [README](deploy/kubernetes/README.md) describes how to deploy UpCloud CSI driver using `kubectl` and sidecar containers.

### HashiCorp Nomad
Nomad deployment [README](deploy/nomad/README.md) describes how to deploy UpCloud CSI driver using Nomad.

## Developing the CSI driver

See [DEVELOPING.md](DEVELOPING.md) for more instructions how to develop and debug UpCloud CSI driver.

## Contribution

Feel free to open PRs and issues, as the development of CSI driver is in progress.
