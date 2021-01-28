# Helm Chart LB-CSI plugin

- [Helm Chart LB-CSI plugin](#helm-chart-lb-csi-plugin)
  - [Usage](#usage)
    - [Install](#install)
    - [Uninstall](#uninstall)
    - [Install in different namespace](#install-in-different-namespace)
    - [Rendering Manifests Using Templates](#rendering-manifests-using-templates)
  - [Values](#values)
  - [Using a custom Docker registry with the Helm Chart](#using-a-custom-docker-registry-with-the-helm-chart)
    - [Custom Docker registry example: Github packages](#custom-docker-registry-example-github-packages)

## Usage

### Install

```bash
helm install --namespace=kube-system lb-csi helm/lb-csi
```

### Uninstall

```bash
helm uninstall --namespace=kube-system helm/lb-csi
```

### Install in different namespace

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

## Values

| name                         | description                                                                         | default         |
|------------------------------|-------------------------------------------------------------------------------------|-----------------|
| discoveryClientInContainer   | Should we deploy lb-nvme-discovery-client as container in lb-csi-node pods          | false           |
| discoveryClientImage         | lb-nvme-discovery-client image name (string format: `<image-name>:<tag>`)           | ""              |
| image                        | lb-csi-plugin image name (string format:  `<image-name>:<tag>`)                     | ""              |
| imageRegistry                |                                                                                     | docker.lightbitslabs.com/lightos-csi|
| imagePullPolicy              |                                                                                     | Always          |
| imagePullSecret              | for more info see [here](#using-a-custom-docker-registry-with-the-helm-chart)       | ""              |
| controllerServiceAccountName | name of controller service account                                                  | lb-csi-ctrl-sa  |
| nodeServiceAccountName       | name of node service account                                                        | lb-csi-node-sa  |
| enableExpandVolume           | Should we allow volume expand feature support (supported for `k8s` v1.16 and above) | true            |
| kubeletRootDir               | Kubelet root directory. (change only k8s deployment is different from default       | /var/lib/kubelet|
| kubeVersion                  | Target k8s version for offline manifests rendering (overrides .Capabilities.Version)| ""              |

## Using a custom Docker registry with the Helm Chart

A custom Docker registry may be used as the source of the operator Docker image. Before "helm install" is run, a Secret of type "docker-registry" should be created with the proper credentials.

Then the `imagePullSecret` helm value may be set to the name of the ImagePullSecret to cause the custom Docker registry to be used.

### Custom Docker registry example: Github packages

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

Now we can run "helm install" with the override value for imagePullSecret. This is often used with an override value for image so that a specific tag can be chosen. Note that the image value should include the full path to the custom registry.

```bash
helm install \
  --set imageRegistry=docker.pkg.github.com/lightbitslabs \
  --set image=lb-csi-plugin:1.4.0 \
  --set imagePullSecrets=github-docker-registry \
  lb-csi ./helm/lb-csi
```
