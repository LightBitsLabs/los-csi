<div style="page-break-after: always;"></div>

## Helm

- [Helm](#helm)
  - [Helm Chart Content](#helm-chart-content)
  - [Chart Values](#chart-values)

[Helm](https://helm.sh/) helps you manage Kubernetes applications — Helm Charts help you define, install, and upgrade even the most complex Kubernetes application.

Helm can be used to install the `lb-csi-plugin`.

The LB-CSI plugin Helm Chart is provided in two ways:

  - [Bundled inside the `lb-csi-bundle-<version>.tar.gz`](./plugin_deployment_using_chart_in_bundle.md)
  - [Lightbits Helm Chart Repository](./plugin_deployment_using_lightbits_helm_repository.md)


### Helm Chart Content

```bash
helm/lb-csi
├── Chart.yaml
├── templates
│   ├── controllerServiceAccount.yaml
│   ├── csidriver.yaml
│   ├── lb-csi-attacher-cluster-role.yaml
│   ├── lb-csi-controller.yaml
│   ├── lb-csi-external-resizer-cluster-role.yaml
│   ├── lb-csi-node.yaml
│   ├── lb-csi-provisioner-cluster-role.yaml
│   ├── nodeServiceAccount.yaml
│   ├── rbac-csi-snapshotter.yaml
│   ├── registry-secret.yml
│   └── secret.yaml
├── values.schema.json
└── values.yaml
```

### Chart Values

| name                               | default                                 | description                                      |
|------------------------------------|-----------------------------------------|--------------------------------------------------|
| discoveryClientInContainer         | false                                   | Deploy lb-nvme-discovery-client as the container in lb-csi-node pods |
| discoveryClientImage               | ""                                      | lb-nvme-discovery-client image name (string format: `<image-name>:<tag>`) |
| maxIOQueues                        | "0"                                     | Overrides the default number of I/O queues created by the driver.<br>Zero value means no override (default driver value is number of cores).  |
| image                              |  ""                                     | lb-csi-plugin image name (string format:  `<image-name>:<tag>`) |
| imageRegistry                      | docker.lightbitslabs.com/lightos-csi    | Registry to pull LightBits CSI images  |
| sidecarImageRegistry               | registry.k8s.io                              | Registry to pull CSI sidecar images                 |
| imagePullPolicy                    | Always                                  |                                                  |
| imagePullSecrets                   | [] (don't use secret)                   | Specify docker-registry secret names as an array. [example](#using-a-custom-docker-registry)  |
| controllerServiceAccountName       | lb-csi-ctrl-sa                          | Name of controller service account                                                  |
| nodeServiceAccountName             | lb-csi-node-sa                          | Name of node service account                                                        |
| enableExpandVolume                 | true                                    | Allow volume expand feature support           |
| enableSnapshotVolume               | true                                    | Allow volume snapshot feature support         |
| kubeletRootDir                     | /var/lib/kubelet                        | Kubelet root directory. (change only k8s deployment is different from default)      |
| kubeVersion                        | ""                                      | Target K8s version for offline manifests rendering (overrides .Capabilities.Version)|
| jwtSecret                          | []                                      | LightOS API JWT to mount as volume for controller and node pods.
| driverNamePrefix                   | []                                      | Provide a custom prefix for the driver. for example if provided - `mydriver` driver will be named: `mydriver.csi.lightbitslabs.com`                    |                    |

