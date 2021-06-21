
## Deploy Block PVC and POD

This Chart will install the following resources:

- A `PVC` named `example-block-pvc` referencing previous defined `StorageClass`
- A `POD` named `example-block-pod` using `example-block-pvc`  

### Deploy Block Workload

```bash
helm install \
  --set block.enabled=true \
  lb-csi-workload-block \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-block
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### Deploy POD On Specific K8S Node

You can choose to deploy the POD on a specific k8s node by specifying `nodeName` or `nodeSelector` in the template.

The default values for these parameters are empty which means POD will not be limited to any POD.

Examples:

1. Specifying `nodeSelector`:

  ```bash
  helm template --set block.enabled=true \
      --set block.nodeSelector."beta\.kubernetes\.io/arch"=amd64,block.nodeSelector.disktype=ssd \
      lb-csi-workload-examples
  ```

  Will result:

  ```bash
  kind: Pod
  apiVersion: v1
  metadata:
    name: example-block-pod
  spec:
    nodeSelector:
      beta.kubernetes.io/arch: amd64
      disktype: ssd
    containers:
    ...
  ```

2. Specifying `nodeName`:

  ```bash
  helm template --set block.enabled=true \
      --set block.nodeName=node00.local \
      lb-csi-workload-examples
  ```

  Will result:

  ```bash
  kind: Pod
  apiVersion: v1
  metadata:
    name: example-block-pod
  spec:
    nodeName: node00.local
    containers:
    ...
  ```

### Verify Block Workload

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                       STORAGECLASS   REASON   AGE
persistentvolume/pvc-2b3b510d-bc4c-4528-a431-3923b8b7d443   3Gi        RWO            Delete           Bound    default/example-block-pvc   example-sc              2m55s

NAME                                      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-block-pvc   Bound    pvc-2b3b510d-bc4c-4528-a431-3923b8b7d443   3Gi        RWO            example-sc     2m56s

NAME                    READY   STATUS    RESTARTS   AGE
pod/example-block-pod   1/1     Running   0          2m56s
```

### Uninstall Block Workload

```bash
helm uninstall lb-csi-workload-block
```

Verify all resources are gone

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```
