<div style="page-break-after: always;"></div>

## Helm

- [Helm](#helm)
  - [Overview](#overview)
  - [Helm Chart Content](#helm-chart-content)
    - [Chart Values](#chart-values)
  - [Install LightOS CSI Plugin](#install-lightos-csi-plugin)
    - [Install In Different Namespace](#install-in-different-namespace)
  - [List Installed Releases](#list-installed-releases)
  - [Uninstall LightOS CSI Plugin](#uninstall-lightos-csi-plugin)
  - [Using A Custom Docker Registry](#using-a-custom-docker-registry)
    - [Custom Docker registry example: Github packages](#custom-docker-registry-example-github-packages)

### Overview

Helm can be used to install the `lb-csi-plugin`.

The LB-CSI plugin Helm chart is provided with `lb-csi-bundle-<version>.tar.gz`.

### Helm Chart Content

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

#### Chart Values

| name                               | default                                 | description                                      |
|------------------------------------|-----------------------------------------|--------------------------------------------------|
| discoveryClientInContainer         | false                                   | Deploy lb-nvme-discovery-client as the container in lb-csi-node pods |
| discoveryClientImage               | ""                                      | lb-nvme-discovery-client image name (string format: `<image-name>:<tag>`) |
| maxIOQueues                        | "0"                                     | Overrides the default number of I/O queues created by the driver.<br>Zero value means no override (default driver value is number of cores).  |
| image                              |  ""                                     | lb-csi-plugin image name (string format:  `<image-name>:<tag>`) |
| imageRegistry                      | docker.lightbitslabs.com/lightos-csi    | Registry to pull LightBits CSI images  |
| sidecarImageRegistry               | quay.io                                 | Registry to pull CSI sidecar images                 |
| imagePullPolicy                    | Always                                  |                                                  |
| imagePullSecrets                   | [] (don't use secret)                   | Specify docker-registry secret names as an array. [example](#using-a-custom-docker-registry)  |
| controllerServiceAccountName       | lb-csi-ctrl-sa                          | Name of controller service account                                                  |
| nodeServiceAccountName             | lb-csi-node-sa                          | Name of node service account                                                        |
| enableExpandVolume                 | true                                    | Allow volume expand feature support (supported for `k8s` v1.16 and above)           |
| enableExpandVolume                 | true                                    | Allow volume snapshot feature support (supported for `k8s` v1.17 and above)         |
| kubeletRootDir                     | /var/lib/kubelet                        | Kubelet root directory. (change only k8s deployment is different from default)      |
| kubeVersion                        | ""                                      | Target K8s version for offline manifests rendering (overrides .Capabilities.Version)|
| jwtSecret                          | []                                      | LightOS API JWT to mount as volume for controller and node pods.                    |


### Install LightOS CSI Plugin

```bash
helm install --namespace=kube-system lb-csi helm/lb-csi
```

#### Install In Different Namespace

You can install the `lb-csi-plugin` in a different namespace (ex: `lb-csi-ns`)
by creating a namespace yourself or using the shortcut to let Helm create a namespace for you:

```bash
helm install -n lb-csi-ns --create-namespace lb-csi helm/lb-csi/
```

### List Installed Releases

```bash
helm list --namespace=kube-system

NAME  	NAMESPACE  	REVISION	UPDATED                                	STATUS  	CHART              	APP VERSION
lb-csi	kube-system	1       	2021-02-11 10:41:57.605518574 +0200 IST	deployed	lb-csi-plugin-0.3.0	1.5.0
```

### Uninstall LightOS CSI Plugin

```bash
helm uninstall --namespace=kube-system lb-csi
```

### Using A Custom Docker Registry

A custom Docker Registry can be used as the source of the container image. Before "helm install" is run, a Secret of type `docker-registry` should be created with the proper credentials.

The secret has to be created in the same namespace where the workload gets deployed.

Then the `imagePullSecrets` Helm value can be set to the name of the `docker-registry` Secret to cause the private Docker Registry to be used.

Both `lb-csi-controller` StatefulSet and `lb-csi-node` DaemonSet use images that might come from a private registry. 

The pod authenticates with the registry using credentials stored in a Kubernetes secret called `github-docker-registry`, which is specified in spec.imagePullSecrets in the name field.

#### Custom Docker registry example: Github packages

Github Packages can be used as a custom Docker registry.

First, a Github personal access token must be created. See instructions [here](https://docs.github.com/en/github/authenticating-to-github/creating-a-personal-access-token)

Second, the access token will be used to create the Secret:

```bash
kubectl create secret docker-registry --namespace kube-system github-docker-registry \
  --docker-username=USERNAME \
  --docker-password=ACCESSTOKEN \
  --docker-server docker.pkg.github.com
```

To see how the secret is stored in Kubernetes, you can use this command:

```bash
kubectl get secret -n kube-system github-docker-registry --output="jsonpath={.data.\.dockerconfigjson}" | base64 --decode
```

Replace `USERNAME` with the github username and `ACCESSTOKEN` with the personal access token.

Now we can run "helm install" with the override value for `imagePullSecrets`. This is often used with an override value for an image so that a specific tag can be selected.

> NOTE:
>
> imagePullSecrets is an array so it should be expressed as such with curly brackets.

```bash
helm install \
  --set imageRegistry=docker.pkg.github.com/lightbitslabs \
  --set image=lb-csi-plugin:1.5.0 \
  --set imagePullSecrets={github-docker-registry} \
  lb-csi ./helm/lb-csi
```
