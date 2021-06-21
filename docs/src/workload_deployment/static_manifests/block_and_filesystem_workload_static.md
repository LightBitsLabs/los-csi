## Dynamic Volume Provisioning Example Using a Pod

A PersistentVolumeClaim is a request for abstract storage resources by a user.
The PersistentVolumeClaim would then be associated to a Pod resource to provision a PersistentVolume, which would be backed by a LightOS volume.

An optional volumeMode can be included to select between a mounted file system (default) or raw block device-based volume.

Using lb-csi-plugin, specifying Filesystem for volumeMode can support ReadWriteOnce accessMode claims, and specifying Block for volumeMode can support ReadWriteOnce accessMode claims.

### Filesystem Volume Mode PVC

The file `examples/filesystem-workload.yaml` provided with the Supplementary Package contains two manifests:

1. PVC named `example-fs-pvc` referencing `example-sc` StorageClass created above.
2. POD named `example-fs-pod` binding to `example-fs-pvc`.

#### Deploy PVC and POD

To deploy the `PVC` and the `POD`, run:

```bash
kubectl apply -f examples/filesystem-workload.yaml
persistentvolumeclaim/example-fs-pvc created
pod/example-fs-pod created
```

#### Verify Deployment

Using the following command we will see the `PV`, `PVC` resources in `Bound` status and `POD` in `READY` state.

```bash
kubectl get pv,pvc,pod
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                    STORAGECLASS   REASON   AGE
persistentvolume/pvc-7680be61-0694-44cf-9d1b-1f69827d0b4b   10Gi       RWO            Delete           Bound    default/example-fs-pvc   example-sc              69s

NAME                                   STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-fs-pvc   Bound    pvc-7680be61-0694-44cf-9d1b-1f69827d0b4b   10Gi       RWO            example-sc     70s

NAME                 READY   STATUS    RESTARTS   AGE
pod/example-fs-pod   1/1     Running   0          70s
```

#### Delete PVC and POD

```bash
kubectl delete -f examples/filesystem-workload.yaml
persistentvolumeclaim "example-fs-pvc" deleted
pod "example-fs-pod" deleted
```

### Block Volume Mode PVC

The file `examples/block-workload.yaml` provided with the Supplementary Package contains two manifests:

1. PVC named `example-block-pvc` referencing `example-sc` StorageClass created above.
2. POD named `example-block-pod` binding to `example-block-pvc`.

You can follow the same flow described in [Filesystem Volume Mode PVC](#filesystem-volume-mode-pvc) to deploy block volume-mode example.

