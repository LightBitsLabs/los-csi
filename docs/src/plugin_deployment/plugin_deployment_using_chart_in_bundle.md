## Installing From Bundled Helm Charts

### Install Lightbits CSI Plugin

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

NAME  	NAMESPACE  	REVISION  UPDATED        	STATUS  	CHART              	 APP VERSION
lb-csi	kube-system	1         2024-12-04... 	deployed	lb-csi-plugin-0.13.0	 1.18.0
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
helm install \
  --set imageRegistry=docker.pkg.github.com/lightbitslabs \
  --set image=lb-csi-plugin:1.18.0 \
  --set imagePullSecrets={github-docker-registry} \
  lb-csi ./helm/lb-csi
```
