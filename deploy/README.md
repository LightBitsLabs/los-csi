# Workload Deployment Examples

The `lb-csi-bundle` includes example manifests to deploy workloads using `lb-csi-plugin` on a k8s cluster.

Part of `lb-csi-plugin` release is the `lb-csi-bundle-<version>.tar.gz`.

Content of the `lb-csi-bundle-<version>.tar.gz`:

```bash
deploy/examples/
│   ├── mt
│   │   ├── example-mt-pod.yaml
│   │   ├── example-mt-pvc.yaml
│   │   ├── example-mt-sc.yaml
│   │   ├── example-mt-sts.yaml
│   │   └── example-secret.yaml
│   └── non-mt
│       ├── block
│       │   ├── example-block-pod.yaml
│       │   └── example-block-pvc.yaml
│       ├── example-sc.yaml
│       ├── example-snapshot-sc.yaml
│       ├── filesystem
│       │   ├── example-fs-pod.yaml
│       │   └── example-fs-pvc.yaml
│       ├── snaps-clones
│       │   ├── 01.example-pvc.yaml
│       │   ├── 02.example-pod.yaml
│       │   ├── 03.example-snapshot.yaml
│       │   ├── 04.example-pvc-from-snapshot.yaml
│       │   ├── 05.example-pvc-from-snapshot-pod.yaml
│       │   ├── 06.example-pvc-from-pvc.yaml
│       │   ├── 07.example-pvc-from-pvc-pod.yaml
│       │   ├── README.md
│       │   ├── test-concurrent-clone.yaml
│       │   └── test-concurrent-snapshot-and-clone.yaml
│       └── statefulset
│           └── example-sts.yaml
```

* **examples:** Examples of workloads that use LightOS CSI Plugin.

## Sample Workload Configurations Using LightOS CSI Plugin

### Create a StorageClass

The Kubernetes StorageClass defines a class of storage. Multiple StorageClass objects can be created to map to different quality-of-service levels and features

For example, to create a lb-csi-plugin StorageClass that maps to the kubernetes pool created above, after ensuring updating the parameters:

* **mgmt-endpoints:** LightOS cluster API endpoints (should be edited to match your LightOS cluster endpoints)
* **replica-count:** the number of replicas for each volume provisioned by this storage class
* **compression:** rather we should enable/disable compression.
* **mgmt-scheme:** access LightOS API using grpc/grpcs (defaults to `grpcs`).

The following YAML file can be used:

```yaml
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: example-sc
provisioner: csi.lightbitslabs.com
allowVolumeExpansion: true
parameters:
  mgmt-endpoint: 10.0.0.1:80,10.0.0.2:80,10.0.0.3:80
  mgmt-scheme: grpc
  replica-count: "3"
  compression: disabled
```

Example file can be found at: [example-sc.yaml](./examples/non-mt/example-sc.yaml)

To create the StorageClass, run:

```bash
kubectl apply -f example-sc.yaml
```

### Sample Configuration For Running Stateful Application Using StatefulSet

This example shows how to consume LightOS filesystem from StatefulSets using the lb-csi-plugin.
Before the example, refer to [StatefulSets](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) for what it is.

For instance, to configure a StatefulSet to provide its pods with `10GiB` persistent storage volumes from the StorageClass described above, you would enter something similar to the following into the spec.volumeClaimTemplates section of the StorageClass spec:

```yaml
  ...
  volumeClaimTemplates:
  - metadata:
      name: test-mnt
    spec:
      accessModes: [ "ReadWriteOnce" ]
      storageClassName: "example-sc"
      resources:
        requests:
          storage: 10Gi
```

An example Kubernetes spec of StatefulSet to create several simple busybox-based pods that use PVs from an “example-sc” StorageClass is provided in the file [example-sts.yaml](./examples/non-mt/statefulset/example-sts.yaml) of the Supplementary Package

To create the StatefulSet, run:

```bash
kubectl apply -f examples/non-mt/statefulset/example-sts.yaml
```

### Create A PersistentVolumeClaim

A PersistentVolumeClaim is a request for abstract storage resources by a user.
The PersistentVolumeClaim would then be associated to a Pod resource to provision a PersistentVolume, which would be backed by a LightOS volume.

An optional volumeMode can be included to select between a mounted file system (default) or raw block device-based volume.

Using lb-csi-plugin, specifying Filesystem for volumeMode can support ReadWriteOnce accessMode claims, and specifying Block for volumeMode can support ReadWriteOnce accessMode claims.

#### Filesystem

For example, to create a mounted filesystem-based PersistentVolumeClaim that utilizes the LightOS CSI Provisioned StorageClass created above, the following YAML can be used to request mounted filesystem volume from the [`example-sc`](#create-a-storageclass) StorageClass:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: example-fs-pvc
spec:
  storageClassName: "example-sc"
  accessModes:
  - ReadWriteOnce
  volumeMode: Filesystem
  resources:
    requests:
      storage: 10Gi
```

Example file can be found at: [example-fs-pvc.yaml](./examples/non-mt/filesystem/example-fs-pvc.yaml)

To create the PVC, run:

```bash
kubectl apply -f example-fs-pvc.yaml
```

The following demonstrates and example of binding the above PersistentVolumeClaim to a Pod resource as mounted filesystem:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: "example-pod"
spec:
  containers:
  - name: busybox-date-container
    imagePullPolicy: IfNotPresent
    image: busybox
    command: ["/bin/sh"]
    args: ["-c", "if [ -f /mnt/test/hostname ] ; then (md5sum -s -c /mnt/test/hostname.md5 && echo OLD MD5 OK || echo BAD OLD MD5) >> /mnt/test/log ; fi ; echo $KUBE_NODE_NAME: $(date +%Y-%m-%d.%H-%M-%S) >| /mnt/test/hostname ; md5sum /mnt/test/hostname >| /mnt/test/hostname.md5 ; echo NEW NODE: $KUBE_NODE_NAME: $(date +%Y-%m-%d.%H-%M-%S) >> /mnt/test/log ; while true ; do date +%Y-%m-%d.%H-%M-%S >| /mnt/test/date ; sleep 10 ; done" ]
    env:
    - name: KUBE_NODE_NAME
      valueFrom:
        fieldRef:
          fieldPath: spec.nodeName
    stdin: true
    tty: true
    volumeMounts:
    - name: test-mnt
      mountPath: "/mnt/test"
  volumes:
  - name: test-mnt
    persistentVolumeClaim:
      claimName: "example-fs-pvc"
```

Example file can be found at: [example-fs-pod.yaml](./examples/non-mt/filesystem/example-fs-pod.yaml)

To create the POD, run:

```bash
kubectl apply -f example-fs-pod.yaml
```
