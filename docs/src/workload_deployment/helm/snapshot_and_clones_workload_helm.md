<div style="page-break-after: always;"></div>
\pagebreak

## Deploy Snapshot and Clones Workloads

This example is a bit more complex and is composed of six different stages:

- [Deploy Snapshot and Clones Workloads](#deploy-snapshot-and-clones-workloads)
  - [_Stage 1: Create `VolumeSnapshotClass`_](#stage-1-create-volumesnapshotclass)
  - [_Stage 2: Create Example `PVC` and `POD`_](#stage-2-create-example-pvc-and-pod)
  - [_Stage 3: Take a `Snapshot` from PVC created at stage 2_](#stage-3-take-a-snapshot-from-pvc-created-at-stage-2)
  - [_Stage 4: Create a `PVC` from Snapshot created at stage 3 and create a `POD` that use it_](#stage-4-create-a-pvc-from-snapshot-created-at-stage-3-and-create-a-pod-that-use-it)
  - [_Stage 5: Create a `PVC` from the `PVC` we created at stage 3 and create a `POD` that use it_](#stage-5-create-a-pvc-from-the-pvc-we-created-at-stage-3-and-create-a-pod-that-use-it)
  - [_Stage 6: Uninstall Snapshot Workloads_](#stage-6-uninstall-snapshot-workloads)

The examples are dependent on one another, so you must run them in order.

For Helm to deploy the `snaps` chart in stages, we introduce the mandatory variable `snaps.stage`
The chart support the following stages:

- snapshot-class
- example-pvc
- snapshot-from-pvc
- pvc-from-snapshot
- pvc-from-pvc


### _Stage 1: Create `VolumeSnapshotClass`_

Create a `VolumeSnapshotClass`:

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=snapshot-class \
  lb-csi-workload-snaps-snapshot-class \
  ./helm/lb-csi-workload-examples
```

### _Stage 2: Create Example `PVC` and `POD`_

Running the following command:

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=example-pvc \
  lb-csi-workload-snaps-example-pvc \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-snaps-example-pvc
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Verify that `PV`, `PVC` are created and in `Bounded` state, and that `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                 STORAGECLASS   REASON   AGE
persistentvolume/pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            Delete           Bound    default/example-pvc   example-sc              57s

NAME                                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-pvc   Bound    pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            example-sc     58s

NAME              READY   STATUS    RESTARTS   AGE
pod/example-pod   1/1     Running   0          58s
```

### _Stage 3: Take a `Snapshot` from PVC created at stage 2_

Create a snapshot from the previously created `PVC` named `example-pvc`

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=snapshot-from-pvc \
  lb-csi-workload-snaps-snapshot-from-pvc \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-snaps-snapshot-from-pvc
LAST DEPLOYED: Tue Feb 23 13:11:16 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Verify that `VolumeSnapshot` and `VolumeSnapshotContent` were created, and `READYTOUSE` status is `true`

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent

NAME                                                      READYTOUSE   SOURCEPVC     SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS         SNAPSHOTCONTENT                                    CREATIONTIME   AGE
volumesnapshot.snapshot.storage.k8s.io/example-snapshot   true         example-pvc                           10Gi          example-snapshot-sc   snapcontent-b710e398-eaa5-45be-bbdc-db74d799e5cc   3m40s          3m49s

NAME                                                                                             READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER                  VOLUMESNAPSHOTCLASS   VOLUMESNAPSHOT     AGE
volumesnapshotcontent.snapshot.storage.k8s.io/snapcontent-b710e398-eaa5-45be-bbdc-db74d799e5cc   true         10737418240   Delete           csi.lightbitslabs.com   example-snapshot-sc   example-snapshot   3m49s
```

### _Stage 4: Create a `PVC` from Snapshot created at stage 3 and create a `POD` that uses it_

Create a `PVC` from the previously taken `Snapshot` named `example-snapshot`

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=pvc-from-snapshot \
  lb-csi-workload-snaps-pvc-from-snapshot \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-snaps-pvc-from-snapshot
LAST DEPLOYED: Tue Feb 23 13:16:26 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Verify that `PV`, `PVC` are created and in `Bounded` state, and that `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pod
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                               STORAGECLASS   REASON   AGE
persistentvolume/pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            Delete           Bound    default/example-pvc                 example-sc              18m
persistentvolume/pvc-d1d20d2b-7fdd-4775-9107-ab8129841a74   10Gi       RWO            Delete           Bound    default/example-pvc-from-snapshot   example-sc              2m24s

NAME                                              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-pvc                 Bound    pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            example-sc     18m
persistentvolumeclaim/example-pvc-from-snapshot   Bound    pvc-d1d20d2b-7fdd-4775-9107-ab8129841a74   10Gi       RWO            example-sc     2m25s

NAME                                READY   STATUS    RESTARTS   AGE
pod/example-pod                     1/1     Running   0          18m
pod/example-pvc-from-snapshot-pod   1/1     Running   0          2m25s
```

> **NOTE:**
>
> We see the `PV`, `PVC` and `POD`s from the Stage #2 as well.

### _Stage 5: Create a `PVC` from the `PVC` we created at stage 3 and create a `POD` that uses it_

Create a `PVC` from the previously taken `Snapshot` named `example-snapshot`

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=pvc-from-pvc \
  lb-csi-workload-snaps-pvc-from-pvc \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-snaps-pvc-from-pvc
LAST DEPLOYED: Tue Feb 23 13:16:26 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Verify that `PV`, `PVC` were created and in `Bounded` state, and that `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pod
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                               STORAGECLASS   REASON   AGE
persistentvolume/pvc-1fe22ed7-3b7e-4094-95a3-995538659c51   10Gi       RWO            Delete           Bound    default/example-pvc-from-pvc        example-sc              15s
persistentvolume/pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            Delete           Bound    default/example-pvc                 example-sc              5h53m
persistentvolume/pvc-d1d20d2b-7fdd-4775-9107-ab8129841a74   10Gi       RWO            Delete           Bound    default/example-pvc-from-snapshot   example-sc              5h36m

NAME                                              STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-pvc                 Bound    pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            example-sc     5h53m
persistentvolumeclaim/example-pvc-from-pvc        Bound    pvc-1fe22ed7-3b7e-4094-95a3-995538659c51   10Gi       RWO            example-sc     23s
persistentvolumeclaim/example-pvc-from-snapshot   Bound    pvc-d1d20d2b-7fdd-4775-9107-ab8129841a74   10Gi       RWO            example-sc     5h36m

NAME                                READY   STATUS    RESTARTS   AGE
pod/example-pod                     1/1     Running   0          5h53m
pod/example-pvc-from-pvc-pod        1/1     Running   0          23s
pod/example-pvc-from-snapshot-pod   1/1     Running   0          5h36m
```

> **NOTE:**
>
> We see the `PV`, `PVC` and `POD`s from the Stages #2 and #4 as well.

### _Stage 6: Uninstall Snapshot Workloads_

If we list the releases installed we will see:

```bash
helm ls
NAME                                    NAMESPACE       REVISION        UPDATED                                 STATUS          CHART                           APP VERSION
lb-csi-workload-sc                      default         1               2021-02-23 08:06:39.915046 +0200 IST    deployed        lb-csi-workload-examples-0.1.0  1.4.0      
lb-csi-workload-snaps-example-pvc       default         1               2021-02-23 12:59:55.458252038 +0200 IST deployed        lb-csi-workload-examples-0.1.0  1.4.0      
lb-csi-workload-snaps-pvc-from-pvc      default         1               2021-02-23 18:52:41.391731359 +0200 IST deployed        lb-csi-workload-examples-0.1.0  1.4.0      
lb-csi-workload-snaps-pvc-from-snapshot default         1               2021-02-23 13:16:26.020897186 +0200 IST deployed        lb-csi-workload-examples-0.1.0  1.4.0      
lb-csi-workload-snaps-snapshot-from-pvc default         1               2021-02-23 13:11:16.746865829 +0200 IST deployed        lb-csi-workload-examples-0.1.0  1.4.0
```

Installation must be in reverse order of the deployment.

After each uninstall we need to verify that all related resources were released before continue to the next uninstall.

Uninstall `pvc-from-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-pvc-from-pvc
```

In order to verify that all resources are deleted, the following command should return no entry:

```bash
kubectl get pv,pvc,pod | grep pvc-from-pvc
```

Uninstall `pvc-from-snapshot`:

```bash
helm uninstall lb-csi-workload-snaps-pvc-from-snapshot
```

In order to verify that all resources are deleted, the following command should return no entry:

```bash
kubectl get pv,pvc,pod | grep pvc-from-snapshot
```

Uninstall `snapshot-from-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-snapshot-from-pvc
```

In order to verify that all resources are deleted, the following command should return no entry:

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent | grep snapshot-from-pvc
```

Uninstall `example-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-example-pvc
```

Verify that all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

Delete `VolumeSnapshotClass`:

```bash
helm uninstall lb-csi-workload-snaps-snapshot-class
```
