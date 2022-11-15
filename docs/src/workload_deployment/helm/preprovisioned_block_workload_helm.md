
## Deploy Pre-provisioned Block PV PVC and POD

Follow these steps to statically create a PVC in a new Kubernetes cluster, using the information from the leftover underlying volume.

> **NOTE:**
>
> If you are reusing a volume from another Kubernetes cluster, delete the PVC and PV objects from the old Kubernetes cluster before creating a PV in the new Kubernetes cluster.

### Deploy Block Workload

This chart will create 3 resources on the cluster:
1. PV pointing to the volume on the LightOS cluster.
2. PVC that will bind to this PV (match the size and labels)
3. POD that will consume this PVC and will run a simple workload on that volume.


```bash
helm install --set preprovisioned.enabled=true \
  --set global.storageClass.mgmtEndpoints="10.20.131.24:443\\,10.20.131.2:443\\,10.20.131.6:443\\,192.168.18.96:443\\,192.168.18.99:443\\,192.168.20.24:443" \
  --set global.jwtSecret.name="example-secret" \
  --set global.jwtSecret.namespace="default" \
  --set preprovisioned.lightosVolNguid=0e436046-4bd2-4d71-a55d-8dc1ea33307c \
  --set preprovisioned.volumeMode=Block \
  --set preprovisioned.storage=1Gi \
  lb-csi-preprovisioned-volume \
  lightbits-helm-repo/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-preprovisioned-volume
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

### Create a PVC that will bind to this new PV

Define the following PVC resource file - `pvc.yml`:

```yaml
kind: PersistentVolumeClaim
apiVersion: v1
metadata:
  name: pre-provisioned-pvc
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
  storageClassName: example-sc
  volumeMode: Block
  volumeName: example-pre-provisioned-pv
```

And create this resource:

```bash
kubectl create -f pvc.yml
```

### Verify PVC is bounded to PV

Verify that 'PV' and 'PVC' are created and in 'Bounded' state, and that 'POD' is in 'Running' state.

```bash
kubectl get pv,pvc,pods
NAME                            CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                                 STORAGECLASS
pv/example-pre-provisioned-pv   1Gi        RWO            Delete           Bound    default/example-pre-provisioned-pvc   example-sc

NAME                              STATUS   VOLUME                       CAPACITY   ACCESS MODES   STORAGECLASS
pvc/example-pre-provisioned-pvc   Bound    example-pre-provisioned-pv   1Gi        RWO            example-sc

NAME                              READY   STATUS    RESTARTS   AGE
pod/example-pre-provisioned-pod   1/1     Running   0          26m
```

### Delete Resources

Delete release:

```bash
helm uninstall lb-csi-preprovisioned-volume
```

Verify that all resources are gone.

```bash
kubectl get pv,pvc
No resources found in default namespace.
```
