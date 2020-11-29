# Deployment bundle

The `lb-csi-bundle` includes example yaml files to deploy `lb-csi-plugin` on a k8s cluster.

The following is the content of the `lb-csi-bundle-<version>.tar.gz`:

```bash
deploy/examples/
├── mt
│   ├── example-mt-pod.yaml
│   ├── example-mt-pvc.yaml
│   ├── example-mt-sc.yaml
│   ├── example-mt-sts.yaml
│   └── example-secret.yaml
└── non-mt
    ├── example-block-pod.yaml
    ├── example-block-pvc.yaml
    ├── example-fs-pod.yaml
    ├── example-fs-pvc.yaml
    ├── example-sc.yaml
    └── example-sts.yaml
```

* **deploy/k8s:** Files to deploy LightOS CSI Plugin on kubernetes
* **examples/k8s:** Examples of kubernetes workloads that use LightOS CSI Plugin.

## Configure LightOS CSI Plugin Deployment

Create the required ServiceAccount and RBAC ClusterRole/ClusterRoleBinding Kubernetes objects

Not every Kubernetes release requires a version-specific deployment spec file.
If the Kubernetes version in question is supported by the Lightbits CSI plugin according to its release notes document, use the deployment spec file targeted for the highest Kubernetes version that is lower or equal than the one being deployed to. E.g., given the above two example deployment files:

* To deploy on Kubernetes v1.13 - use Kubernetes v1.13 deployment spec file.
* To deploy on Kubernetes v1.15 and above - use Kubernetes v1.15 deployment spec file.

### Deploying LightOS CSI Plugin

To deploy the plugin, run the following commands with examples as the current directory and with kubectl in your $PATH.

```bash
kubectl create -f lb-csi-plugin-k8s-v1.15.yaml
```

After the above command completes, the deployment process can take between several seconds and several minutes, depending on the size of the Kubernetes cluster, load on the cluster nodes, network connectivity, etc.

After a short while, you can issue the following commands to verify the results. Your output will likely differ from the following example, including to reflect your Kubernetes cluster configuration, randomly generated pod names, etc.

```bash
$ kubectl get --namespace=kube-system statefulset lb-csi-controller
NAME                DESIRED   CURRENT   AGE
lb-csi-controller   1         1         4m

$ kubectl get --namespace=kube-system daemonsets lb-csi-node
NAME          DESIRED   CURRENT   READY     UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
lb-csi-node   3         3         3         3            3           <none>          4m

$  kubectl get --namespace=kube-system pod --selector app=lb-csi-plugin -o wide
NAME                  READY     STATUS    RESTARTS   AGE       IP              NODE      NOMINATED NODE
lb-csi-controller-0   3/3       Running   0          1m        10.233.65.12    node3     <none>
lb-csi-node-6ptlf     2/2       Running   0          1m        192.168.20.20   node3     <none>
lb-csi-node-blc46     2/2       Running   0          1m        192.168.20.22   node4     <none>
lb-csi-node-djv7t     2/2       Running   0          1m        192.168.20.18   node2     <none>
```

### Removing LightOS CSI Plugin

Assuming you have deployed Lightbits CSI plugin by following the instructions in the section [Deploying LightOS CSI Plugin](#deploying-lightos-csi-plugin), you can remove the CSI plugin from your Kubernetes cluster and confirm the removal by executing the following commands with examples as the current directory.

```bash
$ kubectl delete -f lb-csi-plugin-k8s-v1.15.yaml

$ kubectl get --namespace=kube-system statefulset lb-csi-controller
No resources found.
Error from server (NotFound): statefulsets.apps "lb-csi-controller" not found

$ kubectl get --namespace=kube-system daemonsets lb-csi-node
No resources found.
Error from server (NotFound): daemonsets.extensions "lb-csi-node" not found

$ kubectl get --namespace=kube-system pod --selector app=lb-csi-plugin
No resources found.
```

The “No resources found” errors for the last three commands are expected and confirm the successful removal of the CSI plugin from the Kubernetes cluster.

After Lightbits CSI plugin is removed from the Kubernetes cluster, some volumes created by Kubernetes using the CSI plugin may remain on the LightOS storage cluster and may need to be manually deleted using the LightOS management API or CLI.

## Sample Workload Configurations Using LightOS CSI Plugin

### Create a StorageClass

The Kubernetes StorageClass defines a class of storage. Multiple StorageClass objects can be created to map to different quality-of-service levels and features

For example, to create a lb-csi-plugin StorageClass that maps to the kubernetes pool created above, after ensuring updating the parameters:

* **mgmt-endpoints:** LightOS cluster API endpoints (should be edited to match your LightOS cluster endpoints)
* **replica-count:** the number of replicas for each volume provisioned by this storage class
* **compression:** rather we should enable/disable compression.

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
  replica-count: "3"
  compression: disabled
```

Example file can be found at: [example-sc.yaml](../examples/example-sc.yaml)

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

An example Kubernetes spec of StatefulSet to create several simple busybox-based pods that use PVs from an “example-sc” StorageClass is provided in the file [example-sts.yaml](../examples/stateful-set/example-sts.yaml) of the Supplementary Package

To create the StatefulSet, run:

```bash
kubectl apply -f examples/stateful-set/example-sts.yaml
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

Example file can be found at: [example-fs-pvc.yaml](../examples/pod-fs/example-fs-pvc.yaml)

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

Example file can be found at: [example-fs-pod.yaml](../examples/pod-fs/example-fs-pod.yaml)

To create the POD, run:

```bash
kubectl apply -f example-fs-pod.yaml
```
