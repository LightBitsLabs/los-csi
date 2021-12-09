<div style="page-break-after: always;"></div>

## Lightbits Helm Repository

### Adding Lightbits Helm Repository

```bash
helm repo add lightos-helm-repo https://dl.lightbitslabs.com/public/lightos-csi/helm/charts/
helm repo update
```

### Listing Helm Repositories

```bash
helm repo list
NAME                    URL                                                         
lightos-helm-repo       https://dl.lightbitslabs.com/public/lightos-csi/helm/charts/
```

### Listing All packages from Lightbits Helm Repository

```bash
helm search repo lb-csi
NAME                                            CHART VERSION   APP VERSION     DESCRIPTION
lightos-helm-repo/lb-csi-plugin                 0.5.0           1.7.0           Helm Chart for LightOS CSI Plugin.
lightos-helm-repo/lb-csi-workload-examples      0.5.0           1.7.0           Helm Chart for LightOS CSI Workload Examples.
```

### Deploying release `lb-csi` using `lb-csi-plugin` Chart

Following command will install `lb-csi-plugin` using `lb-csi-plugin` Helm Chart (latest version) from `lightos-helm-repo`.

Notice that we install the plugin under `kube-system` namespace.

```bash
helm install -n kube-system lb-csi lightos-helm-repo/lb-csi-plugin
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
helm show values lightos-helm-repo/lb-csi-plugin 
```

### Deleting Release `lb-csi`

```bash
helm delete lb-csi 
release "lb-csi" uninstalled
```


