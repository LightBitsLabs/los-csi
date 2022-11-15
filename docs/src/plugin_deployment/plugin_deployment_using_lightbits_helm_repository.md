<div style="page-break-after: always;"></div>

## Lightbits Helm Repository

### Adding Lightbits Helm Repository

```bash
helm repo add lightbits-helm-repo https://dl.lightbitslabs.com/public/lightos-csi/helm/charts/
helm repo update
```

### Listing Helm Repositories

```bash
helm repo list
NAME                    URL                                                         
lightbits-helm-repo       https://dl.lightbitslabs.com/public/lightos-csi/helm/charts/
```

### Listing All packages from Lightbits Helm Repository

```bash
helm search repo lightbits-helm-repo
NAME                                            CHART VERSION   APP VERSION     DESCRIPTION
lightbits-helm-repo/lb-csi-plugin                 0.7.1           1.9.1           Helm Chart for LightOS CSI Plugin.
lightbits-helm-repo/lb-csi-workload-examples      0.7.1           1.9.1           Helm Chart for LightOS CSI Workload Examples.
lightbits-helm-repo/snapshot-controller-3         0.7.1           3.0.3           Deploy snapshot-controller for k8s version < v1.20
lightbits-helm-repo/snapshot-controller-4         0.7.1           4.2.1           Deploy snapshot-controller for k8s version >= v1.20
```


### Deploying Snapshot-Controller On Kubernetes Cluster

For reference see: [kubernetes-csi#snapshot-controller](https://kubernetes-csi.github.io/docs/snapshot-controller.html#snapshot-controller)

Volume snapshot is managed by a controller named `Snapshot-Controller`.

Kubernetes admins should bundle and deploy the controller and CRDs as part of their Kubernetes cluster management process (independent of any CSI Driver).

If your cluster does not come pre-installed with the correct components, you may manually install these components by executing these [steps](https://kubernetes-csi.github.io/docs/snapshot-controller.html#deployment)

For convenience we provide Helm Charts to deploy snapshot-controller, CRDs and RBAC rules:

```bash
k8s/
lightbits-helm-repo/snapshot-controller-3         0.7.1           3.0.3           Deploy snapshot-controller for k8s version < v1.20
lightbits-helm-repo/snapshot-controller-4         0.7.1           4.2.1           Deploy snapshot-controller for k8s version >= v1.20
```

Deploy these resources once before installing `lb-csi-plugin`.

> **NOTE:**
>
> If these resources are already deployed for use by other CSI drivers, make sure the versions are correct and skip this step.

### Deploying release `lb-csi` using `lb-csi-plugin` Chart

Following command will install `lb-csi-plugin` using `lb-csi-plugin` Helm Chart (latest version) from `lightbits-helm-repo`.

Notice that we install the plugin under `kube-system` namespace.

```bash
helm install -n kube-system lb-csi lightbits-helm-repo/lb-csi-plugin
NAME: lb-csi
LAST DEPLOYED: Wed Oct 27 11:05:13 2021
NAMESPACE: kube-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### Inspecting the Chart

You can watch the Helm Chart values using the following command:

```bash
helm show values lightbits-helm-repo/lb-csi-plugin 
```

### Deleting Release `lb-csi`

```bash
helm delete lb-csi 
release "lb-csi" uninstalled
```


