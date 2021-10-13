
## Secret and StorageClass Chart

This shart will install the following resources:

- A `Secret` containing the lightos JWT
- A `StorageClass` referencing the secret and configured with all values needed to provision volumes on LightOS.

### Deploy Secret And StorageClasses Workload

```bash
helm install \
  --set storageclass.enabled=true \
  --set global.storageClass.mgmtEndpoints="$MGMT_EP" \
  --set global.jwtSecret.jwt="$LIGHTOS_JWT" \
  lb-csi-workload-examples-sc \
  helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-examples-sc
LAST DEPLOYED: Sun Feb 21 16:12:56 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

> **NOTICE:**
> 
> The chart will validate that the required fields are provided.
> If they are not provided an error will be presented.
>
> For example, if `global.jwtSecret.jwt` is not provided, we will get the following error:
>
> ```bash
> Error: execution error at (lb-csi-workload-examples/charts/storageclass/templates/secret.yaml:1:85): global.jwtSecret.jwt field is required
> ```
> 

### Verify Secret And StorageClasses Workload

Verify that all resources where created:

```bash
kubectl get sc,secret
NAME                                     PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
storageclass.storage.k8s.io/example-sc   csi.lightbitslabs.com   Delete          Immediate           true                   5m27s

NAME                                                       TYPE                                  DATA   AGE
secret/example-secret                                      lightbitslabs.com/jwt                 1      5m27s
```

### Uninstall Secret And StorageClasses Workload

Once done with deployment examples you can delete storageclass resources.

```bash
helm uninstall lb-csi-workload-examples-sc
release "lb-csi-workload-examples-sc" uninstalled
```
