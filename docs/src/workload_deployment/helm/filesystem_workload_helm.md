## Filesystem PVC and POD Workload

This Chart will install the following resources:

- A `PVC` named `example-fs-pvc` referencing previous defined `StorageClass`
- A `POD` named `example-fs-pod` using `example-fs-pvc`  

### Deploy Filesystem Workload

```bash
helm install \
  --set filesystem.enabled=true \
  lb-csi-workload-filesystem \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-filesystem
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
  helm template --set filesystem.enabled=true \
      --set filesystem.nodeSelector."beta\.kubernetes\.io/arch"=amd64,filesystem.nodeSelector.disktype=ssd \
      lb-csi-workload-examples
  ```

  Will result:

  ```bash
  kind: Pod
  apiVersion: v1
  metadata:
    name: example-filesystem-pod
  spec:
    nodeSelector:
      beta.kubernetes.io/arch: amd64
      disktype: ssd
    containers:
    ...
  ```

2. Specifying `nodeName`:

  ```bash
  helm template --set filesystem.enabled=true \
      --set filesystem.nodeName=node00.local \
      lb-csi-workload-examples
  ```

  Will result:

  ```bash
  kind: Pod
  apiVersion: v1
  metadata:
    name: example-fs-pod
  spec:
    nodeName: node00.local
    containers:
    ...
  ```

### Verify Filesystem Workload Deployed

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state:

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                    STORAGECLASS   REASON   AGE
persistentvolume/pvc-e0ad4f63-4b42-417f-8bed-94f8aec8f0d5   10Gi       RWO            Delete           Bound    default/example-fs-pvc   example-sc              33s

NAME                                   STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-fs-pvc   Bound    pvc-e0ad4f63-4b42-417f-8bed-94f8aec8f0d5   10Gi       RWO            example-sc     34s

NAME                 READY   STATUS    RESTARTS   AGE
pod/example-fs-pod   1/1     Running   0          34s
```

### Uninstall Filesystem Workload

```bash
helm uninstall lb-csi-workload-filesystem
```

Verify all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

