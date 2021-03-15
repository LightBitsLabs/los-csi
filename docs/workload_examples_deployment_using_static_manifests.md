# Workload Examples Deployment Using Static Manifests

- [Workload Examples Deployment Using Static Manifests](#workload-examples-deployment-using-static-manifests)
  - [Workload Examples Using Static Manifests Content](#workload-examples-using-static-manifests-content)
  - [Secret and StorageClass](#secret-and-storageclass)
    - [Storing LightOS Authentication JWT in a Kubernetes Secret](#storing-lightos-authentication-jwt-in-a-kubernetes-secret)
    - [Create StorageClass](#create-storageclass)
  - [Dynamic Volume Provisioning Example Using StatefulSet](#dynamic-volume-provisioning-example-using-statefulset)
    - [Deploy StatefulSet](#deploy-statefulset)
    - [Verify StatefulSet Deployment](#verify-statefulset-deployment)
    - [Remove StatefulSet](#remove-statefulset)
  - [Dynamic Volume Provisioning Example Using a Pod](#dynamic-volume-provisioning-example-using-a-pod)
    - [Filesystem Volume Mode PVC](#filesystem-volume-mode-pvc)
      - [Deploy PVC and POD](#deploy-pvc-and-pod)
      - [Verify Deployment](#verify-deployment)
      - [Delete PVC and POD](#delete-pvc-and-pod)
    - [Block Volume Mode PVC](#block-volume-mode-pvc)
    - [Expand Volume Example](#expand-volume-example)
  - [Pre-Provisioned Volume Example Using A Pod](#pre-provisioned-volume-example-using-a-pod)
  - [Volume Snapshot and Clones Provisioning Examples](#volume-snapshot-and-clones-provisioning-examples)
    - [_Stage 1: Create `VolumeSnapshotClass`_](#stage-1-create-volumesnapshotclass)
    - [_Stage 2: Create Example `PVC` and `POD`_](#stage-2-create-example-pvc-and-pod)
    - [_Stage 3: Take a `Snapshot` from PVC created at stage #2_](#stage-3-take-a-snapshot-from-pvc-created-at-stage-2)
    - [_Stage 4: Create a `PVC` from Snapshot created at stage #3 and create a `POD` that use it_](#stage-4-create-a-pvc-from-snapshot-created-at-stage-3-and-create-a-pod-that-use-it)
    - [_Stage 5: Create a `PVC` from the `PVC` we created at stage #4 and create a `POD` that use it_](#stage-5-create-a-pvc-from-the-pvc-we-created-at-stage-4-and-create-a-pod-that-use-it)
    - [_Stage 6: Uninstall Snapshot Workloads_](#stage-6-uninstall-snapshot-workloads)

## Workload Examples Using Static Manifests Content

The following Manifests are provided as part of the Supplementary package:

```bash
examples/
├── secret-and-storage-class.yaml
├── block-workload.yaml
├── filesystem-workload.yaml
├── statefulset-workload.yaml
├── preprovisioned-workload.yaml
├── snaps-example-pvc-workload.yaml
├── snaps-pvc-from-pvc-workload.yaml
├── snaps-pvc-from-snapshot-workload.yaml
└── snaps-snapshot-from-pvc-workload.yaml
```

## Secret and StorageClass

**NOTE:**
> For these workloads to work, you are required at a minimum to provide the following cluster specific parameters:
> - `JWT`
> - `mgmt-endpoints`
>
> Without modifing these parameters, the workloads will likely fail.

### Storing LightOS Authentication JWT in a Kubernetes Secret

Kubernetes and the CSI specification support passing "secrets" when invoking most of the CSI plugin operations, which the plugins can use for authentication and authorization against the corresponding storage provider. This allows a single CSI plugin deployment to serve multiple unrelated users or tenants and to authenticate with the storage provider on their behalf using their credentials, as necessary. The Lightbits CSI plugin takes advantage of this "secrets" passing functionality for authentication and authorization.

**Note:**
> See the LightOS documentation corresponding to the LightOS software version you have deployed for an overview of the security model, details on how to configure the multi-tenancy mode, managing LightOS clusters that have authentication and authorization enabled, uploading credentials to LightOS, generating JSON Web Tokens (`JWT`s), etc.

In order to authenticate on LightOS API a Kubernetes secret containing base64 encoded `JWT` is needed.

Example `JWT` encoding:

```bash
$ export LIGHTOS_JWT=eyJhbGc...lxQ2L7Wpe773w

$ echo -n $LIGHTOS_JWT | base64 -
ZXlKaGJHY2lPaUpTVXpJMU5pSXNJbXRwWkNJNkluTjVjM1JsYlRweWIyOTBJaXdpZEhsd0lqb2lT
bGRVSW4wLmV5SmhkV1FpT2lKTWFXZG9kRTlUSWl3aVpYaHdJam94TmpRMU5UQTNOemcyTENKcFlY
UWlPakUyTVRNNU56RTNPRFlzSW1semN5STZJbk41YzNSbGMzUnpJaXdpYW5ScElqb2lhWFJ5UjNN
Mk1sTk1hMmxhY2xKdlNuWjNXazFhZHlJc0ltNWlaaUk2TVRZeE16azNNVGM0Tml3aWNtOXNaWE1p
T2xzaWMzbHpkR1Z0T21Oc2RYTjBaWEl0WVdSdGFXNGlYU3dpYzNWaUlqb2liR2xuYUhSdmN5MWpi
R2xsYm5RaWZRLlc5QXMwdTJQZnFudTIzZ3U0YXFYcTBKMXZETUJ6bkVfT3dkZkxGeEgzMUdZZVAx
WHFqbUNLUWlZS3pJcXlmcTgweTdCZC02azZvZlVXbzlRZ0FDb1J6LUhRWTJjc1pYdHVHTGRpRzN3
YUF3aEs3QjRIQnhROFAzSnpSeno4TzJLOVg1Z3dRY19xYnpjYTBNaUlrWTZVVjVTOWNEMTROTHNQ
RExwUjdvOFRMbFozbm9kSDZiRlNNVjlPeF9GRXBvTGVidzRWLUlvaURiTV9NdTFDSzZCOUJGeFpN
RTV6NmJIMXlkSDZFWnRuUFlRaUVrRVdlUzFHMUJSTVNfR0hGN3Nja2NYU0c3Q1pkSFFqOHY1b0Y1
YS1USHNVdXR0dmFIc1hUS3FzREFkOHRvbEphZUNUN0NWRFFHX0xUQ1hYZ3dudUI3c0ZRaHJHbHhR
Mkw3V3BlNzczdw==
```

In file `examples/secret-and-storage-class.yaml`, edit `Secret.data.jwt` value with the base64 encoded `JWT` string.

### Create StorageClass

To take advantage of dynamic provisioning, at least one StorageClass Kubernetes object must be created first. A template of the appropriate StorageClass spec looks as follows:

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: <sc-name>
provisioner: csi.lightbitslabs.com
allowVolumeExpansion: <true|false>
parameters:
  mgmt-endpoint: <lb-mgmt-address>:<lb-mgmt-port>[,<lb-mgmt-address>:<lb-mgmt-port>[,...]]
  mgmt-scheme: <grpc|grpcs>
  project-name: <proj-name>
  replica-count: "<num-replicas>"
  compression: <enabled|disabled>
  csi.storage.k8s.io/controller-publish-secret-name: <secret-name>
  csi.storage.k8s.io/controller-publish-secret-namespace: <secret-namespace>
  csi.storage.k8s.io/node-stage-secret-name: <secret-name>
  csi.storage.k8s.io/node-stage-secret-namespace: <secret-namespace>
  csi.storage.k8s.io/node-publish-secret-name: <secret-name>
  csi.storage.k8s.io/node-publish-secret-namespace: <secret-namespace>
  csi.storage.k8s.io/provisioner-secret-name: <secret-name>
  csi.storage.k8s.io/provisioner-secret-namespace: <secret-namespace>
  csi.storage.k8s.io/controller-expand-secret-name: <secret-name>
  csi.storage.k8s.io/controller-expand-secret-namespace: <secret-namespace>
```

You will need to replace the highlighted placeholders (removing the angle brackets in the process) with the actual field values as indicated in the table below. See the LightOS® Administrator's Guide for more information on the LightOS management API service and volume replica counts.

| Placeholder  | Description   |
|--------------|-----------------|
| `<sc-name>` |The name of the StorageClass you want to define. This name will be referenced from other Kubernetes object specs (e.g.: StatefulSet, PersistentVolumeClaim) to use a volume that will be provisioned from a LightOS storage cluster mentioned below, with the corresponding volume characteristics.|
| `<true\|false>`<br>(allowVolumeExpansion) |Kubernetes PersistentVolume-s can be configured to be expandable and LightOS supports volume expansion. If set to true, it will be possible to expand the volumes created from this StorageClass by editing the corresponding PVC Kubernetes objects.<br>**Note:**<br>CSI volumes expansion is enabled in Kubernetes v1.16 and above. CSI volume expansion in older Kubernetes versions is not supported by the Lightbits CSI plugin.|
| `<lb-mgmt-address>` |One of the LightOS management API service endpoint IP addresses of the LightOS cluster on which the volumes belonging to this StorageClass will be created.<br>The mgmt-endpoint entry of the StorageClass spec accepts a comma-separated list of `<lb-mgmt-address>:<lb-mgmt-port>` pairs.<br>For high availability, specify the management API service endpoints of all the LightOS cluster servers, or at least the majority of the servers.|
| `<lb-mgmt-port>` |The port number on which the LightOS management API service is running. Typically, this is port 443 and port 80 for encrypted and encrypted communications, respectively - but LightOS servers can be configured to serve the management interface on other ports as well.|
| `<grpc\|grpcs>` | The protocol to use for communication with the LightOS management API service. LightOS clusters with multi-tenancy support enabled can be accessed only over the TLS-protected grpcs protocol for enhanced security. LightOS clusters with multi-tenancy support disabled may be accessed using the legacy unencrypted grpc protocol.|
| `<proj-name>` | The name of the LightOS project to which the volumes from this StorageClass will belong. The JWT specified using `<secret-name>` below must have sufficient permissions to carry out the necessary actions in that project. |
| `<num-replicas>` | The desired number of replicas for volumes dynamically provisioned for this StorageClass. Valid values are: 1, 2 or 3. The number must be specified in ASCII double quotes (e.g.: "2").|
| `<enabled\|disabled>`| Specifies whether the volumes created for this StorageClass should have compression enabled or disabled. The compression line of the StorageClass spec can be omitted altogether, in which case the LightOS storage cluster default setting for compression will be used. However, if it is present, it must contain one of the following two values: enabled or disabled.|
| `<secret-name>` | The name of the Kubernetes Secret that holds the JWT to be used while making requests pertaining to this StorageClass to the LightOS management API service. See also `<secret-namespace>` below.<br>Typically the JWT used for all the different types of operations (5 in the examples below) will be the same JWT, but there is no requirement for that to be the case.|
| `<secret-namespace>`| The namespace in which the Secret referred to in `<secret-name>` above resides.|

Kubernetes passes the values from the parameters section of the spec verbatim to the Lightbits CSI plugin to inform it of the necessary provisioning actions. Here is an example of a complete StorageClass definition (also available in the file `examples/secret-and-storage-class.yaml` from the Supplementary Package):

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: example-sc
provisioner: csi.lightbitslabs.com
allowVolumeExpansion: true
parameters:
  mgmt-endpoint: 10.10.0.1:443,10.10.0.2:443,10.10.0.3:443
  mgmt-scheme: grpcs
  project-name: default
  replica-count: "3"
  compression: enabled
  csi.storage.k8s.io/controller-publish-secret-name: example-secret
  csi.storage.k8s.io/controller-publish-secret-namespace: default
  csi.storage.k8s.io/node-stage-secret-name: example-secret
  csi.storage.k8s.io/node-stage-secret-namespace: default
  csi.storage.k8s.io/node-publish-secret-name: example-secret
  csi.storage.k8s.io/node-publish-secret-namespace: default
  csi.storage.k8s.io/provisioner-secret-name: example-secret
  csi.storage.k8s.io/provisioner-secret-namespace: default
  csi.storage.k8s.io/controller-expand-secret-name: example-secret
  csi.storage.k8s.io/controller-expand-secret-namespace: default
```

You should modify the `examples/secret-and-storage-class.yaml` file with a list of relevant LightOS management API service endpoints.

To create this StorageClass on the Kubernetes cluster and verify the outcome, you must run the following commands:

```bash
$ kubectl create -f examples/secret-and-storage-class.yaml
secret/example-secret created
storageclass.storage.k8s.io/example-sc created
volumesnapshotclass.snapshot.storage.k8s.io/example-snapshot-sc created

$ kubectl get secret,sc,VolumeSnapshotClass
NAME                         TYPE                                  DATA   AGE
secret/example-secret        kubernetes.io/lb-csi                  1      59s

NAME                                     PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
storageclass.storage.k8s.io/example-sc   csi.lightbitslabs.com   Delete          Immediate           true                   59s

NAME                                                              DRIVER                  DELETIONPOLICY   AGE
volumesnapshotclass.snapshot.storage.k8s.io/example-snapshot-sc   csi.lightbitslabs.com   Delete           59s
```

**NOTE:**
> You can create as many StorageClass-es as you need based on a single or multiple LightOS storage clusters and with different replication factor and compression settings, belonging to the same or different LightOS projects.

## Dynamic Volume Provisioning Example Using StatefulSet

Dynamic PV provisioning is the easiest and most popular way of consuming persistent storage volumes exported from a LightOS storage cluster. In this use case, Kubernetes instructs the Lightbits CSI plugin to create a volume on a specific LightOS storage cluster, and make it available on the appropriate Kubernetes cluster node before a pod requiring the storage is scheduled for execution on that node.

To consume PVs created for a particular StorageClass, the  StorageClass name (not the YAML spec file name) must be referenced within a definition of another Kubernetes object.

For instance, to configure a StatefulSet to provide its pods with 10GiB persistent storage volumes from the StorageClass described above, you would enter something similar to the following into the `StatefulSet.spec.volumeClaimTemplates` section:

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

**Note:**
> Your output will differ, including different IP addresses, Kubernetes cluster nodes, PV and PVC names, and long output lines in the sample output are wrapped to fit the page width.

An example Kubernetes spec of StatefulSet to create several simple busybox-based pods that use PVs from an “example-sc” StorageClass is provided in the file `examples/statefulset-workload.yaml` of the supplementary package.

### Deploy StatefulSet

To create the StatefulSet, run:

```bash
kubectl apply -f examples/statefulset-workload.yaml
statefulset.apps/example-sts created
```

### Verify StatefulSet Deployment

Verify all resources are created and in `READY` state.

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

**Note:**
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

### Expand Volume Example

In order to expand the PV from the “Filesystem VolumeMode Example” to, e.g., 116 GB, the PVC definition needs to be "patched" accordingly:

```bash
kubectl patch pvc example-fs-pvc -p '{"spec":{"resources":{"requests":{"storage": "116Gi" }}}}'
```

Verify that the resize took place, run the following command:

```bash
kubectl describe pvc example-fs-pvc
```

## Pre-Provisioned Volume Example Using A Pod

TBD

## Volume Snapshot and Clones Provisioning Examples

This examples is a bit more complex and is build of six different stages:

   - [_Stage 1: Create `VolumeSnapshotClass`_](#stage-1-create-volumesnapshotclass)
   - [_Stage 2: Create Example `PVC` and `POD`_](#stage-2-create-example-pvc-and-pod)
   - [_Stage 3: Take a `Snapshot` from PVC created at stage #2_](#stage-3-take-a-snapshot-from-pvc-created-at-stage-2)
   - [_Stage 4: Create a `PVC` from Snapshot created at stage #3 and create a `POD` that use it_](#stage-4-create-a-pvc-from-snapshot-created-at-stage-3-and-create-a-pod-that-use-it)
   - [_Stage 5: Create a `PVC` from the `PVC` we created at stage #4 and create a `POD` that use it_](#stage-5-create-a-pvc-from-the-pvc-we-created-at-stage-4-and-create-a-pod-that-use-it)
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

Create a snapshot from previously created `PVC` named `example-pvc`

```bash
kubectl create -f examples/snaps-snapshot-from-pvc-workload.yaml 
volumesnapshot.snapshot.storage.k8s.io/example-snapshot created
```

Verify `VolumeSnapshot` and `VolumeSnapshotContent` were created, and `READYTOUSE` status is `true`

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

Create a `PVC` from previously taken `Snapshot` named `example-snapshot`:

```bash
kubectl create -f examples/snaps-pvc-from-snapshot-workload.yaml 
persistentvolumeclaim/example-pvc-from-snapshot created
pod/example-pvc-from-snapshot-pod created
```

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state.

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

**NOTE:**
> We see the `PV`, `PVC` and `POD`s from the Stage #2 as well.

### _Stage 5: Create a `PVC` from the `PVC` we created at stage #4 and create a `POD` that use it_

Create a `PVC` from previously taken `Snapshot` named `example-snapshot`

```bash
kubectl create -f examples/snaps-pvc-from-pvc-workload.yaml 
persistentvolumeclaim/example-pvc-from-pvc created
pod/example-pvc-from-pvc-pod created
```

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state.

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

**NOTE:**
> We see the `PV`, `PVC` and `POD`s from the Stages #2 and #4 as well.

### _Stage 6: Uninstall Snapshot Workloads_

Installation MUST be in reverse order of the deployment.

After each uninstall we need to verify that all related resources were released before continue to next uninstall.

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

Verify all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

Delete `VolumeSnapshotClass`:

```bash
kubectl delete -f examples/snaps-example-snapshot-class.yaml
volumesnapshotclass "example-snapshot-sc" deleted
```
