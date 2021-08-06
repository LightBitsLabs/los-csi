## Dynamic Volume Provisioning Example Using StatefulSet

Dynamic PV provisioning is the easiest and most popular way of consuming persistent storage volumes exported from a LightOS storage cluster. In this use case, Kubernetes instructs the Lightbits CSI plugin to create a volume on a specific LightOS storage cluster, and make it available on the appropriate Kubernetes cluster node before a pod requiring the storage is scheduled for execution on that node.

To consume PVs created for a particular StorageClass, the  StorageClass name (not the YAML spec file name) must be referenced within a definition of another Kubernetes object.

For instance, to configure a StatefulSet to provide its pods with 10GB of persistent storage volumes from the StorageClass described above, you would enter something similar to the following into the `StatefulSet.spec.volumeClaimTemplates` section:

```yaml
    ...
  volumeClaimTemplates:
  - metadata:
      name: lb-pvc
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "example-sc"
      resources:
        requests:
          storage: 10Gi
```

An example Kubernetes spec of StatefulSet to create several simple busybox-based pods that use PVs from an `example-sc` StorageClass is provided in the file `examples/statefulset-workload.yaml` of the Supplementary Package.

The commands and their respective outputs in the following shell transcript illustrate the creation of a StatefulSet from this spec and the corresponding Kubernetes object created.

> **Note:**
>
> Your output will differ, including different IP addresses, Kubernetes cluster nodes, PV and PVC names, and long output lines in the sample output are wrapped to fit the page width.

An example Kubernetes spec of StatefulSet to create several simple busybox-based pods that use PVs from an “example-sc” StorageClass is provided in the file `examples/statefulset-workload.yaml` of the supplementary package.

### Deploy StatefulSet

To create the StatefulSet, run:

```bash
kubectl apply -f examples/statefulset-workload.yaml
statefulset.apps/example-sts created
```

### Verify StatefulSet Deployment

Verify that all resources are created and in `READY` state.

```bash
kubectl get statefulset example-sts
NAME          READY   AGE
example-sts   3/3     86s
```

```bash
kubectl get pods --selector app=example-sts-app -o wide
NAME            READY   STATUS    RESTARTS   AGE    IP            NODE                   NOMINATED NODE   READINESS GATES
example-sts-0   1/1     Running   0          118s   10.244.1.10   rack08-server62-vm06   <none>           <none>
example-sts-1   1/1     Running   0          111s   10.244.2.8    rack11-server97-vm03   <none>           <none>
example-sts-2   1/1     Running   0          105s   10.244.1.11   rack08-server62-vm06   <none>           <none>
```

```bash
kubectl get pvc
NAME                     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
test-mnt-example-sts-0   Bound    pvc-9458c28e-59b2-4311-8fcf-14d121773b51   10Gi       RWO            example-sc     2m20s
test-mnt-example-sts-1   Bound    pvc-8da4e3fb-04dc-4ea5-8870-5f2b853b1cf4   10Gi       RWO            example-sc     2m13s
test-mnt-example-sts-2   Bound    pvc-cfff7d93-77bd-4dec-acc8-4fbf1e28b949   10Gi       RWO            example-sc     2m7s
```

```bash
kubectl get pv
NAME                                       CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                            STORAGECLASS   REASON   AGE
pvc-8da4e3fb-04dc-4ea5-8870-5f2b853b1cf4   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-1   example-sc              2m50s
pvc-9458c28e-59b2-4311-8fcf-14d121773b51   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-0   example-sc              2m57s
pvc-cfff7d93-77bd-4dec-acc8-4fbf1e28b949   10Gi       RWO            Delete           Bound    default/test-mnt-example-sts-2   example-sc              2m43s
```

As can be seen above, three pods belonging to the `example-sts` StatefulSet were created, with three corresponding PVs and `PVC`s.

To confirm that a pod has a PV attached to it and mounted, you can execute the following command and then cross-reference the output against the listings of PVs and `PVC`s, as shown in the previous sample output.

```bash
kubectl describe pod example-sts-0
Name:         example-sts-0
    ...
Status:       Running
IP:           10.244.1.10
IPs:
  IP:           10.244.1.10
Controlled By:  StatefulSet/example-sts
Containers:
  busybox-date-cont:
    ...
    Mounts:
      /mnt/test from test-mnt (rw)
    ...
Volumes:
  test-mnt:
    Type:       PersistentVolumeClaim (a reference to a PersistentVolumeClaim in the same namespace)
    ClaimName:  test-mnt-example-sts-0
    ReadOnly:   false
    ...
Events:
  Type     Reason                  Age                    From                           Message
  ----     ------                  ----                   ----                           -------
  ...
  kubelet, rack08-server62-vm06  Created container busybox-date-cont
  Normal   Started                 6m33s                  kubelet, rack08-server62-vm06  Started container busybox-date-cont
```

Based on the “Mounts:” section of the output example, you can see how the PV is represented inside a pod by executing the following command:

```bash
kubectl exec -it example-sts-0 -- /bin/sh -c "mount | grep /tmp/demo"
/dev/nvme0n3 on /tmp/demo type ext4 (rw,relatime,data=ordered)
```

Most of the mountpoints of the container running inside the pod above are either virtual file systems or file systems rooted in Docker overlay mounts. However, the LightOS-backed volume mountpoint will be a direct mount of the NVMe device exported to the Kubernetes node over NVMe/TCP transport (here: /dev/nvme0n3).

To see the corresponding underlying volumes information of the LightOS storage server, you can connect to the LightOS server using SSH. Then, use the LightOS CLI utility (lbcli, see LightOS® Administrator's Guide for details) to execute the following command:

```bash
lbcli list volumes
Name                                       UUID                                   State       Protection State   NSID      Size      Replicas   Compression   ACL              Rebuild Progress
pvc-094081e6-80b8-11e9-a4ab-a4bf014dfc95   091f644d-df47-47ff-9b62-13c3a5df3542   Available   FullyProtected     4         10 GiB    3          false         values:"ACL3"    None
pvc-f7d33e21-80b7-11e9-a4ab-a4bf014dfc95   99ff3144-ae34-414a-b1d0-0b1a84919aaf   Available   FullyProtected     2         10 GiB    3          false         values:"ACL1"    None
pvc-03b5e41e-80b8-11e9-a4ab-a4bf014dfc95   f9bcfe15-fbe3-4697-80f7-77cd9f1e54f0   Available   FullyProtected     3         10 GiB    3          false         values:"ACL2"    None
```

When the LightOS CSI plugin creates a volume on the LightOS storage cluster, it uses the name of the `PV` that Kubernetes passed to it as the LightOS volume name. Therefore, you can also cross-reference the volume names in the `lbcli` output against the kubectl pv command output you obtained earlier.

### Remove StatefulSet

Finally, to remove the StatefulSet and all of the corresponding pods created by the instructions above, as well as to verify their removal, execute the following commands:

```bash
kubectl delete -f examples/statefulset-workload.yaml
statefulset.apps "example-sts" deleted
```

```bash
$ kubectl get pods --selector app=example-sts-app -o wide
No resources found.
```

Due to the way that StatefulSet-s are currently implemented in Kubernetes, the `PV`s and `PVC`s created by the above instructions—and the LightOS volumes backing them—will likely remain for reasons detailed in the following note.

> **Note:**
>
> Kubernetes has a number of features designed to avert user data loss by preventing PV and/or PVC deletion in circumstances that might otherwise seem unexpected to someone unfamiliar with Kubernetes workings. These include:
>
> - Reclaim policy of PVs (see the “Reclaiming” section of the Kubernetes PVs documentation for details).
> - Protection of in-use PVs/PVCs (see the “Storage Object in Use Protection” section of the Kubernetes PVs documentation for details).
StatefulSet PVC protection (see Kubernetes GitHub issue #55045)
>
> The general Kubernetes troubleshooting links in the previous note might help in finding the root cause for the PV/PVC not being deleted as expected.

_**Please make sure you delete leftover `PVC` and `PV` resources before you proceed by running the following commands:**_

```bash
kubectl delete \
  persistentvolumeclaim/test-mnt-example-sts-0 \
  persistentvolumeclaim/test-mnt-example-sts-1 \
  persistentvolumeclaim/test-mnt-example-sts-2

persistentvolumeclaim "test-mnt-example-sts-0" deleted
persistentvolumeclaim "test-mnt-example-sts-1" deleted
persistentvolumeclaim "test-mnt-example-sts-2" deleted
```

Verify all `PV`, `PVC` and `POD`s are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```
