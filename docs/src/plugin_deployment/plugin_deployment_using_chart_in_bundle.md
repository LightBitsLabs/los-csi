## Installing From Bundled Helm Charts

The bundle holds the charts as packaged archives in `helm/charts/`:

```bash
helm/charts/
├── lb-csi-plugin-<chart-version>.tgz
├── lb-csi-workload-examples-<chart-version>.tgz
└── snapshot-controller-<chart-version>.tgz
```

Extract the [Lightbits CSI Bundle Package](./deployment.md#download-lightbits-csi-bundle-package) and run the commands on this page from the directory you extracted it into:

```bash
mkdir lb-csi-bundle && tar -C lb-csi-bundle -xzf lb-csi-bundle-<version>.tar.gz
cd lb-csi-bundle
```

> **NOTE:**
>
> Give Helm the path of the `.tgz` file. Helm reads a chart name such as `helm/lb-csi` as `<repository>/<chart>` and fails with `Error: INSTALLATION FAILED: repo helm not found`.

### Install Snapshot-Controller

Volume snapshots need the snapshot-controller and its CRDs. Install them once, before you install `lb-csi-plugin`:

```bash
helm install --namespace=kube-system snapshot-controller helm/charts/snapshot-controller-*.tgz
```

> **NOTE:**
>
> If these resources are already deployed for use by other CSI drivers, make sure the versions are correct and skip this step.

### Install Lightbits CSI Plugin

```bash
helm install --namespace=kube-system lb-csi helm/charts/lb-csi-plugin-*.tgz
```

#### Install In Different Namespace

You can install the `lb-csi-plugin` in a different namespace (ex: `lb-csi-ns`)
by creating a namespace yourself or using the shortcut to let Helm create a namespace for you:

```bash
helm install -n lb-csi-ns --create-namespace lb-csi helm/charts/lb-csi-plugin-*.tgz
```

### List Installed Releases

```bash
helm list --namespace=kube-system

NAME  	NAMESPACE  	REVISION  UPDATED        	STATUS  	CHART              	 APP VERSION
lb-csi	kube-system	1         2026-10-06... 	deployed	lb-csi-plugin-0.22.0	 v1.24.0
```

### Uninstall Lightbits CSI Plugin

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
helm install --namespace=kube-system \
  --set imageRegistry=docker.pkg.github.com/lightbitslabs \
  --set image=lb-csi-plugin:v1.20.0 \
  --set imagePullSecrets={github-docker-registry} \
  lb-csi helm/charts/lb-csi-plugin-*.tgz
```
