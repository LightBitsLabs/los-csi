<div style="page-break-after: always;"></div>
\pagebreak

## Upgrade CSI Plugin Procedure

- [Upgrade CSI Plugin Procedure](#upgrade-csi-plugin-procedure)
  - [Upgrade Overview](#upgrade-overview)
  - [Applying Manual Upgrade](#applying-manual-upgrade)
    - [Stage #1: Modify DaemonSet's `spec.updateStrategy` to `OnDelete`](#stage-1-modify-daemonsets-specupdatestrategy-to-ondelete)
    - [Stage #2: Update DaemonSet `lb-csi-plugin` Image](#stage-2-update-daemonset-lb-csi-plugin-image)
    - [Stage #3: Select One Node Apply Upgrade And Verify](#stage-3-select-one-node-apply-upgrade-and-verify)
    - [Stage #4: Verify Upgraded `lb-csi-node` `POD` Functioning Properly](#stage-4-verify-upgraded-lb-csi-node-pod-functioning-properly)
      - [Verify Upgraded Node Using Static Manifests](#verify-upgraded-node-using-static-manifests)
      - [Verify Upgraded Node Using Helm](#verify-upgraded-node-using-helm)
    - [Stage #5: Upgrade Remaining `lb-csi-node` `POD`s](#stage-5-upgrade-remaining-lb-csi-node-pods)
    - [Stage #6: Modify DaemonSet's `spec.updateStrategy` back to `RollinUpdate`](#stage-6-modify-daemonsets-specupdatestrategy-back-to-rollinupdate)
    - [Stage #7: Upgrade StatefulSet](#stage-7-upgrade-statefulset)
    - [Verify StatefulSet And DaemonSet Version As Expected](#verify-statefulset-and-daemonset-version-as-expected)
  - [Applying RollingUpgrade (Automated Deployment)](#applying-rollingupgrade-automated-deployment)
    - [Checking DaemonSet Update Strategy](#checking-daemonset-update-strategy)
    - [Checking StatefulSet Update Strategy](#checking-statefulset-update-strategy)
    - [Rollout History](#rollout-history)
    - [Rollout Status](#rollout-status)
    - [Verify StatefulSet And DaemonSet Version As Expected](#verify-statefulset-and-daemonset-version-as-expected-1)
    - [Rollback DaemonSet](#rollback-daemonset)
    - [Rollback StatefulSet](#rollback-statefulset)
  - [Verify Upgraded Cluster Is Working](#verify-upgraded-cluster-is-working)


> NOTE:
> 
> Since we specify `spec.template.spec.priorityClassName = system-cluster-critical` we should get rescheduled 
> even if the server is low on resources. [see here](https://kubernetes.io/docs/tasks/administer-cluster/guaranteed-scheduling-critical-addon-pods/)
> 
> From [pod-priority-preemption](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption), we can see that the priority-class is instructing the server to preempt lower priority PODs if needed.

> NOTE:
> 
> On Production deployments we would want to do a node upgrade manually, to verify that there is no service loss.

### Upgrade Overview

K8s supports two ways for upgrading resources:

- `OnDelete` - once a `POD` is deleted the new scheduled `POD` will be running with upgraded spec. Using this strategy we can choose which `POD` will be upgraded and we have mode control over the flow.
- `RollingUpgrade` - Once applied K8s will do the upgrade of the DaemonSet one by one on its own, without the ability to intervene if something goes wrong.

We will prefer the manual approach to make sure there is no service loss while upgrading.

This is the flow we recommend for upgrading the CSI plugin:

1. Upgrade the `lb-csi-node` DaemonSet `POD`s manually, one by one.
2. Verify that the upgraded node is still working.
3. Upgrade the `lb-csi-controller` StatefulSet.
4. Verify that the entire cluster is working.

### Applying Manual Upgrade

Manual flow:

   1. [Stage #1: Modify DaemonSet's `spec.updateStrategy` to `OnDelete`](#stage-1-modify-daemonsets-specupdatestrategy-to-ondelete)
   2. [Stage #2: Select One Node And Apply Upgrade](#stage-2-select-one-node-and-apply-upgrade)
   3. [Stage #3: Verify Updated POD Functioning Properly](#stage-3-verify-updated-pod-functioning-properly)
   4. [Stage #4: Upgrade Remaining `lb-csi-node` `POD`s](#stage-4-upgrade-remaining-lb-csi-node-pods)
   5. [Stage #5: Modify DaemonSet's `spec.updateStrategy` back to `RollinUpdate`](#stage-5-modify-daemonsets-specupdatestrategy-back-to-rollinupdate)
   6. [Stage #6: Upgrade StatefulSet](#stage-6-upgrade-statefulset)

#### Stage #1: Modify DaemonSet's `spec.updateStrategy` to `OnDelete`

```bash
kubectl patch ds/lb-csi-node -n kube-system -p '{"spec":{"updateStrategy":{"type":"OnDelete"}}}'
daemonset.apps/lb-csi-node patched

# verify changes applied
kubectl get ds/lb-csi-node -o go-template='{{.spec.updateStrategy.type}}{{"\n"}}' -n kube-system
OnDelete
```

#### Stage #2: Update DaemonSet `lb-csi-plugin` Image

The only difference between the two `DaemonSet`s is the `lb-csi-plugin` image:

  ```bash
  <  image: docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.2.0
  ---
  >  image: docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.4.2
  ```

In case the discovery-client is deployed as a container in `lb-csi-node` POD we should add the following difference as well:

  ```bash
  <  image: docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.2.0
  ---
  >  image: docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.4.2
  ```

> **Note:**
>
> The Docker registry prefix may vary between deployments.

Updating only the container image, use `kubectl set image`

  ```bash
  kubectl set image ds/lb-csi-node -n kube-system lb-csi-plugin=docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.4.2
  ```

  In case discovery-client is deployed as a container in `lb-csi-node` POD, run the following command as well:

  ```bash
  kubectl set image ds/lb-csi-node -n kube-system lb-nvme-discovery-client=docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.4.2
  ```

#### Stage #3: Select One Node Apply Upgrade And Verify

We will specify how to manually upgrade the image in each of the PODs:

1. List all the `lb-csi-plugin` pods in the cluster:

  ```bash
  kubectl get pods -n kube-system -l app=lb-csi-plugin -o wide
  NAME                  READY   STATUS    RESTARTS   AGE     IP               NODE                   NOMINATED NODE   READINESS GATES
  lb-csi-controller-0   4/4     Running   0          117m    10.244.3.7       rack06-server63-vm04   <none>           <none>
  lb-csi-node-rwrz6     3/3     Running   0          5m10s   192.168.20.61    rack06-server63-vm04   <none>           <none>
  lb-csi-node-stzg6     3/3     Running   0          5m      192.168.20.84    rack06-server67-vm03   <none>           <none>
  lb-csi-node-wc46m     3/3     Running   0          17h     192.168.16.114   rack09-server69-vm01   <none>           <none>
  ```

  For this example, select the first `lb-csi-node`:

  ```bash
  NAME                  READY   STATUS    RESTARTS   AGE     IP               NODE                   NOMINATED NODE   READINESS GATES
  lb-csi-node-rwrz6     3/3     Running   0          5m10s   192.168.20.61    rack06-server63-vm04   <none>           <none>
  ```

1. Delete the POD running on our selected server.

  ```bash
  kubectl delete pods/lb-csi-node-rwrz6 -n kube-system 
  pod "lb-csi-node-rwrz6" deleted
  ```

4. Verify that the `lb-csi-node` POD is upgraded.

  Listing the PODs again will show that one of them has a very short Age and it would have a different name:

   ```bash
   kubectl get pods -n kube-system -l app=lb-csi-plugin -o wide
   NAME                  READY   STATUS    RESTARTS   AGE    IP               NODE                   NOMINATED NODE   READINESS GATES
   lb-csi-node-g47z2     2/2     Running   0          39s    192.168.20.61    rack06-server63-vm04   <none>           <none>
   ```

   We need to verify that it is `Running`.

   We should also verify that the image was updated correctly by running the following command:

   ```bash
   kubectl get pods lb-csi-node-g47z2 -n kube-system -o jsonpath='{.spec.containers[?(@.name=="lb-csi-plugin")].image}' ; echo
   docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.4.2
   ```

   In case discovery-client is deployed as a container in the `lb-csi-node` POD, verify that its image was updated as well with the following command:

   ```bash
   kubectl get pods lb-csi-node-tpd7d -n kube-system -o jsonpath='{.spec.containers[?(@.name=="lb-nvme-discovery-client")].image}' ; echo
   docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:1.4.2
   ```

#### Stage #4: Verify Upgraded `lb-csi-node` `POD` Functioning Properly

We will run a simple verification test to see that our node is still functioning before we move to the next node.

By deploy a simple workload **on the upgraded node**, we can verify that the `lb-csi-node` node is functioning properly.

We provide two ways to run the verification test:

1. [Using Static Manifests](#verify-upgraded-node-using-static-manifests)
2. [Using Provided Helm Chart](#verify-upgraded-node-using-helm)

##### Verify Upgraded Node Using Static Manifests

Our verification test is very simple and has the following steps:

  1. Create an example `PVC`.
  2. Deploy a `POD` consuming this `PVC` **on upgraded node**.

Create a manifest file named `fs-workload.yaml` containing the two kinds we want to deploy - `PVC` and `POD`:

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: example-fs-after-upgrade-pvc
spec:
  storageClassName: "<STORAGE-CLASS-NAME>"
  accessModes:
  - ReadWriteOnce
  volumeMode: Filesystem
  resources:
    requests:
      storage: 10Gi
---
apiVersion: v1
kind: Pod
metadata:
  name: "example-fs-after-upgrade-pod"
spec:
  nodeName: "<NODE-NAME>"
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
      claimName: "example-fs-after-upgrade-pvc"
```

Make sure you modify the following fields that are cluster specific:

- `storageClassName`: name of the SC configured in your cluster.
- `nodeName`: name of the node we want to deploy on.
- `Pod.spec.image`: name of the busybox image. docker registry prefix may vary between deployments.

In order to get this, run the following commands:

```bash
kubectl get pods -n kube-system -l app=lb-csi-plugin -o wide
NAME                  READY   STATUS    RESTARTS   AGE     IP               NODE                   NOMINATED NODE   READINESS GATES
lb-csi-controller-0   4/4     Running   0          117m    10.244.3.7       rack06-server63-vm04   <none>           <none>
lb-csi-node-rwrz6     3/3     Running   0          17h     192.168.20.61    rack06-server63-vm04   <none>           <none>
lb-csi-node-stzg6     3/3     Running   0          5m      192.168.20.84    rack06-server67-vm03   <none>           <none>
lb-csi-node-wc46m     3/3     Running   0          17h     192.168.16.114   rack09-server69-vm01   <none>           <none>
```

We can see that POD `lb-csi-node-stzg6` was the one that had restarted and was updated, so we will set `nodeName` to be `rack06-server67-vm03`.

Apply the following command:

```bash
kubectl create -f fs-workload.yaml
```

The workload will write some files to the mounted volume and you can run the following command to see the content is written to the volume:

```bash
kubectl exec -it pod/example-fs-after-upgrade-pod -- /bin/sh -c "cat /mnt/test/date ; cat /mnt/test/hostname; cat /mnt/test/hostname.md5"
2021-05-23.08-13-10
rack08-server52: 2021-05-23.08-03-30
61afe45d31f826f5b7e54e6bd92ec07d  /mnt/test/hostname
```

After a successful workload on the upgraded node, delete the tmp workload by running:

```bash
kubectl delete -f fs-workload.yaml
```

##### Verify Upgraded Node Using Helm

We will use the workload Helm chart provided with the bundle for this:

```bash
kubectl get storageclass
NAME  PROVISIONER            RECLAIMPOLICY  BINDINGMODE ALLOWVOLUMEEXPANSION AGE
lb-sc csi.lightbitslabs.com  Delete         Immediate   false                2d12h
```

We will use the name of the `StorageClass` and the name of the upgraded node (`rack06-server63-vm04`) to deploy the FS pod workload.

```bash
helm install --set filesystem.enabled=true \
   --set global.storageClass.name=lb-sc \
   --set filesystem.nodeName=rack06-server63-vm04 \
   fs-workload \
   lb-csi-workload-examples
```

Now we need to verify that the PVC was `Bound` and that the `POD` is in `Ready` status.

```bash
kubectl get pv,pvc,pod
NAME                                                       STATUS CLAIM                  SC    AGE
persistentvolume/pvc-6b26b875-fafd-4abe-95bb-2f5305b61a29  Bound  default/example-fs-pvc lb-sc 12m

NAME                                  STATUS  VOLUME                                    SC    AGE
persistentvolumeclaim/example-fs-pvc  Bound   pvc-6b26b875-fafd-4abe-95bb-2f5305b61a29  lb-sc 12m

NAME                    READY   STATUS    RESTARTS   AGE
pod/example-fs-pod      1/1     Running   0          12m
```

If all is well we can assume the upgrade for that node worked.

Now we will uninstall the workload using the command:

```bash
helm delete fs-workload
```

#### Stage #5: Upgrade Remaining `lb-csi-node` `POD`s

Repeat the following steps:

* [Stage #3: Select One Node Apply Upgrade And Verify](#stage-3-select-one-node-apply-upgrade-and-verify)
* [Stage #4: Verify Upgraded `lb-csi-node` `POD` Functioning Properly](#stage-4-verify-upgraded-lb-csi-node-pod-functioning-properly)

#### Stage #6: Modify DaemonSet's `spec.updateStrategy` back to `RollinUpdate`

```bash
kubectl patch ds/lb-csi-node -n kube-system -p '{"spec":{"updateStrategy":{"type":"RollingUpdate"}}}'
daemonset.apps/lb-csi-node patched

# verify changes applied
kubectl get ds/lb-csi-node -o go-template='{{.spec.updateStrategy.type}}{{"\n"}}' -n kube-system
RollingUpdate
```

#### Stage #7: Upgrade StatefulSet

Since we have only one replica in the `lb-csi-controller` `StatefulSet` there is no need to do a rolling upgrade.

Between the two spoken versions there were many modifications to the StatefulSet, since we added Snapshots.

Snapshot requires the following resources to be deployed on the K8s cluster:

1. Snapshot RBAC `ClusterRole` and `ClusterRoleBinding`s.
2. Custom resource definitions:
   1. kind: VolumeSnapshot (version: `v0.4.0`, apiVersion: `apiextensions.k8s.io/v1`)
   2. kind: VolumeSnapshotClass (version: `v0.4.0`, apiVersion: `apiextensions.k8s.io/v1`)
   3. VolumeSnapshotContent (version: `v0.4.0`, apiVersion: `apiextensions.k8s.io/v1`)
3. Two additional containers in the `lb-csi-controller` POD:
   1. name: snapshot-controller  (v4.0.0)
   2. name: csi-snapshotter      (v4.0.0)

1. Deploy ClusterRole and ClusterRoleBindings

  > **NOTE:**
  >
  > We assume that the K8s cluster admin will know what is deployed on the system.
  
  The following steps allow us to validate if we have the Roles and Bindings to work with snapshots.
  
  If resources are not present on the cluster, these steps will guide you how to add them.
  
  1. Verify if we have `ClusterRole`s for snapshots:
  
    ```bash
    kubectl get clusterrole  | grep snap
    ```
  
    If we get empty response we will need to deploy the ClusterRoles, see step #3.
  
    If we get the following output:
  
    ```bash
    external-snapshotter-runner       2d15h
    snapshot-controller-runner        2d15h
    ```
  
    It means that the roles are deployed and the Cluster-Admin need to make sure that the granted permissions are enough.
  
  2. The same should be done with the ClusterRoleBindings:
  
    ```bash
    kubectl get clusterrolebindings | grep snap
    ```
  
    If we get empty response we will need to deploy the ClusterRoleBindings, see step #3.
  
    If we get the following output:
  
    ```bash
    csi-snapshotter-role        2d15h
    snapshot-controller-role    2d15h
    ```
  
    It means that the roles are deployed and the Cluster-Admin need to make `ClusterRoleBinding`s are assigned to the correct `ServiceAccount`.
  
  3. Deploy `ClusterRoles` and `ClusterRoleBinding` using the following command
  
    ```bash
    kubectl create -f snapshot-rbac.yaml 
    clusterrole.rbac.authorization.k8s.io/snapshot-controller-runner created
    clusterrole.rbac.authorization.k8s.io/external-snapshotter-runner created
    clusterrolebinding.rbac.authorization.k8s.io/snapshot-controller-role created
    clusterrolebinding.rbac.authorization.k8s.io/csi-snapshotter-role created
    ```

2. Deploy Snapshot CRDs

  We need to understand if we have the snapshot `CRD`s deployed already on the cluster.
  
  ```bash
  kubectl get crd -o jsonpath='{range .items[*]}{@.spec.names.kind}{" , "}{@.apiVersion}{" , "}{@.metadata.annotations.controller-gen\.kubebuilder\.io/version}{"\n"}{end}' ;echo
  ```
  
  If we see output like this, we already have `CRD` deployed on the cluster and we can skip adding them:
  
  ```bash
  VolumeSnapshotClass , apiextensions.k8s.io/v1 , v0.4.0
  VolumeSnapshotContent , apiextensions.k8s.io/v1 , v0.4.0
  VolumeSnapshot , apiextensions.k8s.io/v1 , v0.4.0
  ```
  
  If we get no output it means that we do not have CRDs deployed and we need to deploy them as follows:
  
  ```bash
  kubectl create -f snapshot-crds.yaml 
  customresourcedefinition.apiextensions.k8s.io/volumesnapshotclasses.snapshot.storage.k8s.io created
  customresourcedefinition.apiextensions.k8s.io/volumesnapshotcontents.snapshot.storage.k8s.io created
  customresourcedefinition.apiextensions.k8s.io/volumesnapshots.snapshot.storage.k8s.io created
  ```

3. Upgrade `lb-csi-controller` `StatefulSet`

  > **Note:**
  >
  > The Docker registry prefix may vary between deployments. Please verify the image prefix before running.
  
  ```bash
  kubectl apply -f stateful-set.yaml
  Warning: kubectl apply should be used on resource created by either kubectl create --save-config or kubectl apply
  statefulset.apps/lb-csi-controller configured
  ```

#### Verify StatefulSet And DaemonSet Version As Expected

List all CSI plugin pods:

```bash
kubectl get pods -n kube-system -l app=lb-csi-plugin
NAME                  READY   STATUS    RESTARTS   AGE
lb-csi-controller-0   6/6     Running   0          3m33s
lb-csi-node-k4bzk     3/3     Running   0          13m
lb-csi-node-pcsmm     3/3     Running   0          13m
lb-csi-node-z7lpr     3/3     Running   0          13m
```

Verify that the `version-rel` matches the expected version.

For the controller pod: 

```bash
kubectl logs -n kube-system lb-csi-controller-0 -c lb-csi-plugin | grep version-rel
time="2021-03-21T18:50:54.410655+00:00" level=info msg=starting config="{NodeID:rack06-server63-vm04.ctrl Endpoint:unix:///var/lib/csi/sockets/pluginproxy/csi.sock DefaultFS:ext4 LogLevel:debug LogRole:controller LogTimestamps:true LogFormat:text BinaryName: Transport:tcp SquelchPanics:true PrettyJson:false}" driver-name=csi.lightbitslabs.com node=rack06-server63-vm04.ctrl role=controller version-build-id= version-git$
v1.4.2-0-gaf08f7e0 version-hash=1.4.2 version-rel=1.4.2
```

The same for each node pod:

```bash
kubectl logs -n kube-system lb-csi-node-k4bzk -c lb-csi-plugin | grep version-rel
time="2021-03-21T18:41:18.750957+00:00" level=info msg=starting config="{NodeID:rack06-server63-vm04.node Endpoint:unix:///csi/csi.sock DefaultFS:ext4 LogLevel:debug LogRole:node LogTimestamps:true LogFormat:text BinaryName: Transport:tcp SquelchPanics:true PrettyJson:false}" driver-name=csi.lightbitslabs.com node=rack06-server63-vm04.node role=node version-build-id= version-git=v1.4.2-0-gaf08f7e0 version-hash=1.4.2 version-rel=1.4.2
```

### Applying RollingUpgrade (Automated Deployment)

#### Checking DaemonSet Update Strategy

```bash
kubectl get ds/lb-csi-node -o go-template='{{.spec.updateStrategy.type}}{{"\n"}}' -n kube-system
```

#### Checking StatefulSet Update Strategy

```bash
kubectl get sts/lb-csi-controller -o go-template='{{.spec.updateStrategy.type}}{{"\n"}}' -n kube-system
```

#### Rollout History

Each time we deploy the DaemonSet a new `rollout` will be created.
This can be viewed using the following command:

```bash
kubectl rollout history daemonset lb-csi-node -n kube-system
daemonset.apps/lb-csi-node 
REVISION  CHANGE-CAUSE
1         <none>
```

The same can be seen for ReplicaSet resources:

```bash
kubectl rollout history statefulset lb-csi-controller -n kube-system
statefulset.apps/lb-csi-controller 
REVISION
1
2
```

#### Rollout Status

We can verify the status of a rollout using the following command:

```bash
kubectl rollout status daemonset lb-csi-node -n kube-system
daemon set "lb-csi-node" successfully rolled out
```

#### Verify StatefulSet And DaemonSet Version As Expected

List all CSI plugin pods:

```bash
kubectl get pods -n kube-system -l app=lb-csi-plugin
NAME                  READY   STATUS    RESTARTS   AGE
lb-csi-controller-0   6/6     Running   0          3m33s
lb-csi-node-k4bzk     2/2     Running   0          13m
lb-csi-node-pcsmm     2/2     Running   0          13m
lb-csi-node-z7lpr     2/2     Running   0          13m
```

Verify that the `version-rel` matches the expected version.

For the controller pod: 

```bash
kubectl logs -n kube-system lb-csi-controller-0 -c lb-csi-plugin | grep version-rel
time="2021-03-21T18:50:54.410655+00:00" level=info msg=starting config="{NodeID:rack06-server63-vm04.ctrl Endpoint:unix:///var/lib/csi/sockets/pluginproxy/csi.sock DefaultFS:ext4 LogLevel:debug LogRole:controller LogTimestamps:true LogFormat:text BinaryName: Transport:tcp SquelchPanics:true PrettyJson:false}" driver-name=csi.lightbitslabs.com node=rack06-server63-vm04.ctrl role=controller version-build-id= version-git$
v1.4.2-0-gaf08f7e0 version-hash=1.4.2 version-rel=1.4.2
```

The same for each node pod:

```bash
kubectl logs -n kube-system lb-csi-node-k4bzk -c lb-csi-plugin | grep version-rel
time="2021-03-21T18:41:18.750957+00:00" level=info msg=starting config="{NodeID:rack06-server63-vm04.node Endpoint:unix:///csi/csi.sock DefaultFS:ext4 LogLevel:debug LogRole:node LogTimestamps:true LogFormat:text BinaryName: Transport:tcp SquelchPanics:true PrettyJson:false}" driver-name=csi.lightbitslabs.com node=rack06-server63-vm04.node role=node version-build-id= version-git=v1.4.2-0-gaf08f7e0 version-hash=1.4.2 version-rel=1.4.2
```

#### Rollback DaemonSet

In case nothing works, we can roll back.

```bash
kubectl rollout undo daemonset lb-csi-node -n kube-system 
daemonset.apps/lb-csi-node rolled back
```

Now we can see again that the rollout has changed and that we got a new ControllerRevision (Always incrementing):

```bash
kubectl rollout history daemonset lb-csi-node -n kube-system daemonset.apps lb-csi-node
REVISION  CHANGE-CAUSE                                      
2         <none> 
3         <none>                                  
```

#### Rollback StatefulSet

```bash
kubectl rollout undo statefulset lb-csi-controller -n kube-system 
statefulset.apps/lb-csi-controller rolled back
```

```bash
kubectl rollout history statefulset lb-csi-controller -n kube-system
statefulset.apps/lb-csi-controller 
REVISION
2
3
```

### Verify Upgraded Cluster Is Working

Once we are done with all operations for the upgrade, we should run different workloads to verify that all is functioning properly:

1. Create block PVC,POD
2. Create filesystem PVC,POD
3. Create snapshots, clones, clone `PVC`s

We can use the workload examples provided with the `lb-csi-bundle-<version>.tar.gz` of the target version.
