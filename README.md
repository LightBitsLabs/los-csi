# lb-csi plugin

- [lb-csi plugin](#lb-csi-plugin)
  - [Deployment On Kubernetes](#deployment-on-kubernetes)
    - [Plugin Deployment Using Static Manifest](#plugin-deployment-using-static-manifest)
    - [Plugin Deployment Using Helm](#plugin-deployment-using-helm)
    - [Workload Deployment Examples](#workload-deployment-examples)
  - [Design and architecture](#design-and-architecture)
  - [Developing The Plugin](#developing-the-plugin)
    - [Requirements](#requirements)

## Deployment On Kubernetes

Part of `lb-csi-plugin` release is the `lb-csi-bundle-<version>.tar.gz`.

Content of the `lb-csi-bundle-<version>.tar.gz`:

```bash
deploy/k8s/
deploy/helm/
deploy/examples/
```

- **k8s:** Contains static manifests to deploy LightOS CSI Plugin, on different k8s cluster versions.
- **helm:** Contains helm charts to deploy LightOS CSI Plugin, on different k8s cluster versions.
- **examples:** Contains examples of workloads that use LightOS CSI Plugin.

### Plugin Deployment Using Static Manifest

For more information on plugin deployment using provided manifests see [here](deploy/README.md)

### Plugin Deployment Using Helm

For more information on Helm deployment see [this](deploy/helm/lb-csi/README.md)

### Workload Deployment Examples

We provide deployment examples for `lb-csi-plugin` and workloads that utilize it [here](deploy/README.md)

## Design and architecture

See [docs/design.md](./docs/design.md)

## Developing The Plugin

### Requirements

In order to build the plugin you will need:

- docker.
- golang >=1.14.
- helm >= 3.5.0.

See [docs/develop.md](./docs/develop.md)
