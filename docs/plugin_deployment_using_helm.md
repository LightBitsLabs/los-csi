# LightOS CSI Plugin Deployment Using Helm

- [LightOS CSI Plugin Deployment Using Helm](#lightos-csi-plugin-deployment-using-helm)
  - [Overview](#overview)
  - [Helm Chart Content](#helm-chart-content)
    - [Chart Values](#chart-values)
  - [Usage](#usage)
    - [Install LightOS CSI Plugin](#install-lightos-csi-plugin)
    - [List Installed Releases](#list-installed-releases)
    - [Uninstall LightOS CSI Plugin](#uninstall-lightos-csi-plugin)
    - [Install In Different Namespace](#install-in-different-namespace)
    - [Rendering Manifests Using Templates](#rendering-manifests-using-templates)
    - [Using A Custom Docker Registry](#using-a-custom-docker-registry)
      - [Custom Docker registry example: Github packages](#custom-docker-registry-example-github-packages)
  - [Next Steps](#next-steps)

## Overview

Helm may be used to install the `lb-csi-plugin`.

LB-CSI plugin Helm chart is provided with `lb-csi-bundle-<version>.tar.gz`.

## Helm Chart Content

```bash
├── helm
│   └── lb-csi
│       ├── Chart.yaml
│       ├── templates
│       │   ├── controllerServiceAccount.yaml
│       │   ├── csidriver.yaml
│       │   ├── csinodeinfo_crd.yaml
│       │   ├── lb-csi-attacher-cluster-role.yaml
│       │   ├── lb-csi-controller.yaml
│       │   ├── lb-csi-external-resizer-cluster-role.yaml
│       │   ├── lb-csi-node.yaml
│       │   ├── lb-csi-provisioner-cluster-role.yaml
│       │   ├── nodeServiceAccount.yaml
│       │   ├── registry-secret.yml
│       │   ├── secret.yaml
│       │   ├── snapshot-rbac.yaml
│       │   ├── volume-snapshot-class-crd.yaml
│       │   ├── volume-snapshot-content-crd.yaml
│       │   └── volume-snapshot-crd.yaml
│       └── values.yaml
```

### Chart Values

| name                         | description                                                                         | default         |
|------------------------------|-------------------------------------------------------------------------------------|-----------------|
| discoveryClientInContainer   | Should we deploy lb-nvme-discovery-client as container in lb-csi-node pods          | false           |
| discoveryClientImage         | lb-nvme-discovery-client image name (string format: `<image-name>:<tag>`)           | ""              |
| image                        | lb-csi-plugin image name (string format:  `<image-name>:<tag>`)                     | ""              |
| imageRegistry                | registry to pull LightBits CSI images                           | docker.lightbitslabs.com/lightos-csi|
| sidecarImageRegistry         | registry to pull CSI sidecar images                                                 | quay.io         |
| imagePullPolicy              |                                                                                     | Always          |
| imagePullSecrets             | for more info see [here](#using-a-custom-docker-registry-with-the-helm-chart)       | []              |
| controllerServiceAccountName | name of controller service account                                                  | lb-csi-ctrl-sa  |
| nodeServiceAccountName       | name of node service account                                                        | lb-csi-node-sa  |
| enableExpandVolume           | Should we allow volume expand feature support (supported for `k8s` v1.16 and above) | true            |
| kubeletRootDir               | Kubelet root directory. (change only k8s deployment is different from default       | /var/lib/kubelet|
| kubeVersion                  | Target k8s version for offline manifests rendering (overrides .Capabilities.Version)| ""              |

## Usage

### Install LightOS CSI Plugin

```bash
helm install --namespace=kube-system lb-csi helm/lb-csi
```

### List Installed Releases

```bash
helm list --namespace=kube-system

NAME  	NAMESPACE  	REVISION	UPDATED                                	STATUS  	CHART              	APP VERSION
lb-csi	kube-system	1       	2021-02-11 10:41:57.605518574 +0200 IST	deployed	lb-csi-plugin-0.1.0	1.4.0
```

### Uninstall LightOS CSI Plugin

```bash
helm uninstall --namespace=kube-system lb-csi
```

### Install In Different Namespace

You can install the `lb-csi-plugin` in a different namespace (ex: `lb-csi-ns`)
by creating a namespace your self or using the shortcut to let helm create a namespace for you:

```bash
helm install -n lb-csi-ns --create-namespace lb-csi helm/lb-csi/
```

### Rendering Manifests Using Templates

Render manifests to folder `/tmp/helm/lb-csi-plugin-k8s-v1.15` run following command:

```bash
helm template deploy/helm/lb-csi/ \
  --set enableExpandVolume=true \
  --set kubeVersion=v1.15 \
  --output-dir=/tmp/helm/lb-csi-plugin-k8s-v1.15
```

Render manifests to file `lb-csi-plugin-k8s-v1.15.yaml` run following command:

```bash
helm template deploy/helm/lb-csi/ \
  --set enableExpandVolume=true \
  --set kubeVersion=v1.15 \
  --set enableSnapshot=true > lb-csi-plugin-k8s-v1.15.yaml
```

Render manifest not on k8s cluster to target specific kubernetes version:

```bash
helm template deploy/helm/lb-csi/ \
  --set enableExpandVolume=true \
  --set kubeVersion=v1.17.0 \
  --set enableSnapshot=true > lb-csi-plugin-k8s-v1.17.yaml
```

### Using A Custom Docker Registry

A custom Docker registry may be used as the source of the operator Docker image. Before "helm install" is run, a Secret of type "docker-registry" should be created with the proper credentials.

Then the `imagePullSecrets` helm value may be set to the name of the ImagePullSecret to cause the custom Docker registry to be used.

#### Custom Docker registry example: Github packages

Github Packages may be used as a custom Docker registry.

First, a Github personal access token must be created. See instructions [here](https://docs.github.com/en/github/authenticating-to-github/creating-a-personal-access-token)

Second, the access token will be used to create the Secret:

```bash
kubectl create secret docker-registry github-docker-registry \
  --docker-username=USERNAME \
  --docker-password=ACCESSTOKEN \
  --docker-server docker.pkg.github.com
```

Replace `USERNAME` with the github username and `ACCESSTOKEN` with the personal access token.

Now we can run "helm install" with the override value for `imagePullSecrets`. This is often used with an override value for image so that a specific tag can be chosen.

```bash
helm install \
  --set imageRegistry=docker.pkg.github.com/lightbitslabs \
  --set image=lb-csi-plugin:1.4.0 \
  --set imagePullSecrets=github-docker-registry \
  lb-csi ./helm/lb-csi
```

## Next Steps

- [Workload Examples Deployment Using Static Manifests](./workload_examples_deployment_using_static_manifests.md).
- [Workload Examples Deployment Using Helm](./workload_examples_deployment_using_helm.md).
