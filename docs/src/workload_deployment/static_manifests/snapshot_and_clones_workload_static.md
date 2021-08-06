## Volume Snapshot and Clones Provisioning Examples

This example is a bit more complex and is composed of six different stages:

   - [_Stage 1: Create `VolumeSnapshotClass`_](#stage-1-create-volumesnapshotclass)
   - [_Stage 2: Create Example `PVC` and `POD`_](#stage-2-create-example-pvc-and-pod)
   - [_Stage 3: Take a `Snapshot` from PVC created at stage #2_](#stage-3-take-a-snapshot-from-pvc-created-at-stage-2)
   - [_Stage 4: Create a `PVC` from Snapshot created at stage #3 and create a `POD` that uses it_](#stage-4-create-a-pvc-from-snapshot-created-at-stage-3-and-create-a-pod-that-use-it)
   - [_Stage 5: Create a `PVC` from the `PVC` we created at stage #4 and create a `POD` that uses it_](#stage-5-create-a-pvc-from-the-pvc-we-created-at-stage-4-and-create-a-pod-that-use-it)
   - [_Stage 6: Uninstall Snapshot Workloads_](#stage-6-uninstall-snapshot-workloads)

The examples are dependent on one another, so you must run them in order.

### _Stage 1: Create `VolumeSnapshotClass`_

Create a `VolumeSnapshotClass`:

```bash
kubectl create -f examples/snaps-example-snapshot-class.yaml
```

### _Stage 2: Create Example `PVC` and `POD`_

Running the following command:

```bash
kubectl create -f examples/snaps-example-pvc-workload.yaml
persistentvolumeclaim/example-pvc created
pod/example-pod created
```

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                 STORAGECLASS   REASON   AGE
persistentvolume/pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            Delete           Bound    default/example-pvc   example-sc              57s

NAME                                STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-pvc   Bound    pvc-797e45c0-2333-47ce-9a5e-2bb46b101163   10Gi       RWO            example-sc     58s

NAME              READY   STATUS    RESTARTS   AGE
pod/example-pod   1/1     Running   0          58s
```

### _Stage 3: Take a `Snapshot` from PVC created at stage #2_

Create a snapshot from the previously created `PVC` named `example-pvc`

```bash
kubectl create -f examples/snaps-snapshot-from-pvc-workload.yaml 
volumesnapshot.snapshot.storage.k8s.io/example-snapshot created
```

Verify `VolumeSnapshot` and `VolumeSnapshotContent` were created, and that `READYTOUSE` status is `true`

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent

NAME                                                      READYTOUSE   SOURCEPVC     SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS         SNAPSHOTCONTENT                                    CREATIONTIME   AGE
volumesnapshot.snapshot.storage.k8s.io/example-snapshot   true         example-pvc                           10Gi          example-snapshot-sc   snapcontent-3be9e67b-ece7-4f08-8c40-922a4e84247c   2s             7s

NAME                                                              DRIVER                  DELETIONPOLICY   AGE
volumesnapshotclass.snapshot.storage.k8s.io/example-snapshot-sc   csi.lightbitslabs.com   Delete           81s
```

### _Stage 4: Create a `PVC` from Snapshot created at stage #3 and create a `POD` that use it_

After your VolumeSnapshot object is bound, you can use that object to provision a new volume that is pre-populated with data from the snapshot.

The volume snapshot content object is used to restore the existing volume to a previous state.

Create a `PVC` from the previously taken `Snapshot` named `example-snapshot`:

```bash
kubectl create -f examples/snaps-pvc-from-snapshot-workload.yaml 
persistentvolumeclaim/example-pvc-from-snapshot created
pod/example-pvc-from-snapshot-pod created
```

Verify that `PV`, `PVC` were created and in `Bounded` state, and that `POD` is in `Running` state.

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

### _Stage 5: Create a `PVC` from the `PVC` we created at stage #4 and create a `POD` that use it_

Create a `PVC` from the previously taken `Snapshot` named `example-snapshot`

```bash
kubectl create -f examples/snaps-pvc-from-pvc-workload.yaml 
persistentvolumeclaim/example-pvc-from-pvc created
pod/example-pvc-from-pvc-pod created
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

Installation MUST be in reverse order of the deployment.

After each uninstall we need to verify that all related resources were released before continuing to the next uninstall.

Uninstall `pvc-from-pvc`:

```bash
kubectl delete -f examples/snaps-pvc-from-pvc-workload.yaml
persistentvolumeclaim "example-pvc-from-pvc" deleted
pod "example-pvc-from-pvc-pod" deleted
```

In order to verify that all resources are deleted, the following command should not generate any output:

```bash
kubectl get pv,pvc,pod | grep pvc-from-pvc
```

Uninstall `pvc-from-snapshot`:

```bash
kubectl delete -f examples/snaps-pvc-from-snapshot-workload.yaml 
persistentvolumeclaim "example-pvc-from-snapshot" deleted
pod "example-pvc-from-snapshot-pod" deleted
```

In order to verify that all resources are deleted, the following command should not generate any output:

```bash
kubectl get pv,pvc,pod | grep pvc-from-snapshot
```

Uninstall `snapshot-from-pvc`:

```bash
kubectl delete -f examples/snaps-snapshot-from-pvc-workload.yaml 
volumesnapshot.snapshot.storage.k8s.io "example-snapshot" deleted
```

In order to verify that all resources are deleted, the following command should not generate any output:

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent | grep snapshot-from-pvc
```

Uninstall `example-pvc`:

```bash
kubectl delete -f examples/snaps-example-pvc-workload.yaml 
persistentvolumeclaim "example-pvc" deleted
pod "example-pod" deleted
```

Verify that all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

Delete `VolumeSnapshotClass`:

```bash
kubectl delete -f examples/snaps-example-snapshot-class.yaml
volumesnapshotclass "example-snapshot-sc" deleted
```
