# LightOS CSI Deployment

- [LightOS CSI Deployment](#lightos-csi-deployment)
  - [Download LightOS CSI Bundle Package](#download-lightos-csi-bundle-package)
  - [LightOS CSI Bundle Package Content](#lightos-csi-bundle-package-content)
  - [Next Steps](#next-steps)

## Download LightOS CSI Bundle Package

Lightbits supplies an optional supplementary package that contains the configuration files used for Lightbits CSI plugin deployment, as well as some Persistent Volume usage example files.

The link to the supplementary package should be similar to:

```bash
curl -l -s -O https://dl.lightbitslabs.com/public/lightos-csi/raw/files/lb-csi-bundle-<version>.tar.gz
```

## LightOS CSI Bundle Package Content

The `lb-csi-bundle` includes the following content:

```bash
├── k8s
├── examples
├── helm
│   ├── lb-csi
│   ├── lb-csi-workload-examples
```

- **k8s:** Contains static manifests to deploy `lb-csi-plugin` on various Kubertnetes versions.
- **examples:** Provides various workload examples that use `lb-csi` as persistent storage backend using static manifests.
- **helm/lb-csi:** Provides a customizable way to deploy `lb-csi-plugin` using Helm on various Kubertnetes versions using Helm Chart.
- **helm/lb-csi-workload-examples:** Provides various workload examples that use `lb-csi` as persistent storage backend using Helm Chart.

> **NOTE:** The following sections use these supplementary package files for demonstration purposes.

## Next Steps

We provide two ways of deploying applications on Kubernetes:

- Using Static Manifests:
  - [LightOS CSI Plugin Deployment Using Static Manifests](./plugin_deployment_static_manifests.md).
  - [Workload Examples Deployment Using Static Manifests](./workload_examples_deployment_using_static_manifests.md).
- Using Helm:
  - [LightOS CSI Plugin Deployment Using Helm](./plugin_deployment_using_helm.md).
  - [Workload Examples Deployment Using Helm](./workload_examples_deployment_using_helm.md).
