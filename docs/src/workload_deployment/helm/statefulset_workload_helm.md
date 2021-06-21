
## Deploy StatefulSet

### Deploy StatefulSet Workload

```bash
helm install \
  --set statefulset.enabled=true \
  lb-csi-workload-sts \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-sts
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### Verify StatefulSet Workload

Verify following conditions are met:

- `PV` and `PVC` is `Bound`
- `POD`s status is `Running`
- `StatefulSet` has 3/3 `Ready`

```bash
kubectl get pv,pvc,pods,sts
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                            STORAGECLASS   REASON   AGE
persistentvolume/pvc-1a16e8da-427a-47a1-9974-c7f18b9d8abb   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-0   example-sc              2m4s
persistentvolume/pvc-57e6e555-43e3-4f39-9433-c287d3ab53d6   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-1   example-sc              114s
persistentvolume/pvc-945f0393-e711-4c75-b2f9-b80222d346ab   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-2   example-sc              103s

NAME                                           STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/test-mnt-example-sts-0   Bound    pvc-1a16e8da-427a-47a1-9974-c7f18b9d8abb   10Gi       RWO            example-sc     2m5s
persistentvolumeclaim/test-mnt-example-sts-1   Bound    pvc-57e6e555-43e3-4f39-9433-c287d3ab53d6   10Gi       RWO            example-sc     116s
persistentvolumeclaim/test-mnt-example-sts-2   Bound    pvc-945f0393-e711-4c75-b2f9-b80222d346ab   10Gi       RWO            example-sc     104s

NAME                READY   STATUS    RESTARTS   AGE
pod/example-sts-0   1/1     Running   0          2m5s
pod/example-sts-1   1/1     Running   0          116s
pod/example-sts-2   1/1     Running   0          104s

NAME                           READY   AGE
statefulset.apps/example-sts   3/3     2m5s
```

### Uninstall StatefulSet Workload

```bash
helm uninstall lb-csi-workload-sts
```

Verify `StatefulSet` and `POD`s resources are gone:

```bash
kubectl get pv,pvc,pods,sts
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                            STORAGECLASS   REASON   AGE
persistentvolume/pvc-1a16e8da-427a-47a1-9974-c7f18b9d8abb   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-0   example-sc              5m31s
persistentvolume/pvc-57e6e555-43e3-4f39-9433-c287d3ab53d6   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-1   example-sc              5m21s
persistentvolume/pvc-945f0393-e711-4c75-b2f9-b80222d346ab   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-2   example-sc              5m10s

NAME                                           STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/test-mnt-example-sts-0   Bound    pvc-1a16e8da-427a-47a1-9974-c7f18b9d8abb   10Gi       RWO            example-sc     5m32s
persistentvolumeclaim/test-mnt-example-sts-1   Bound    pvc-57e6e555-43e3-4f39-9433-c287d3ab53d6   10Gi       RWO            example-sc     5m23s
persistentvolumeclaim/test-mnt-example-sts-2   Bound    pvc-945f0393-e711-4c75-b2f9-b80222d346ab   10Gi       RWO            example-sc     5m11s
```

Since the default `StorageClass.reclaimPolicy` is `Retain` the `PVC`s and `PV`s will remain and not be deleted.

In order to delete them run the following:

```bash
kubectl delete \
  persistentvolumeclaim/test-mnt-example-sts-0 \
  persistentvolumeclaim/test-mnt-example-sts-1 \
  persistentvolumeclaim/test-mnt-example-sts-2

persistentvolumeclaim "test-mnt-example-sts-0" deleted
persistentvolumeclaim "test-mnt-example-sts-1" deleted
persistentvolumeclaim "test-mnt-example-sts-2" deleted
```

Verify all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```