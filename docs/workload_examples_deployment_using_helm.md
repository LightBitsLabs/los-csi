# Workload Examples Deployment Using Helm

Helm chart ease the deployment of the provided workload examples that use the `lb-csi-plugin` as a persistant storage backend.

- [Workload Examples Deployment Using Helm](#workload-examples-deployment-using-helm)
  - [Overview](#overview)
  - [Helm Chart Content](#helm-chart-content)
    - [Chart Values](#chart-values)
      - [Mandatory Values To Modify](#mandatory-values-to-modify)
  - [Usage](#usage)
    - [Secret, StorageClass and VolumeSnapshotClass Chart](#secret-storageclass-and-volumesnapshotclass-chart)
      - [Deploy Secret And StorageClasses Workload](#deploy-secret-and-storageclasses-workload)
      - [Verify Secret And StorageClasses Workload](#verify-secret-and-storageclasses-workload)
      - [Uninstall Secret And StorageClasses Workload](#uninstall-secret-and-storageclasses-workload)
    - [Deploy Block PVC and POD](#deploy-block-pvc-and-pod)
      - [Deploy Block Workload](#deploy-block-workload)
      - [Verify Block Workload](#verify-block-workload)
      - [Uninstall Block Workload](#uninstall-block-workload)
    - [Filesystem PVC and POD Workload](#filesystem-pvc-and-pod-workload)
      - [Deploy Filesystem Workload](#deploy-filesystem-workload)
      - [Verify Filesystem Workload Deployed](#verify-filesystem-workload-deployed)
      - [Uninstall Filesystem Workload](#uninstall-filesystem-workload)
    - [Deploy StatefulSet](#deploy-statefulset)
      - [Deploy StatefulSet Workload](#deploy-statefulset-workload)
      - [Verify StatefulSet Workload](#verify-statefulset-workload)
      - [Uninstall StatefulSet Workload](#uninstall-statefulset-workload)
    - [Deploy Snapshot and Clones Workloads](#deploy-snapshot-and-clones-workloads)
      - [_Stage 1: Create Example `PVC` and `POD`_](#stage-1-create-example-pvc-and-pod)
      - [_Stage 2: Take a `Snapshot` from PVC created at stage 1_](#stage-2-take-a-snapshot-from-pvc-created-at-stage-1)
      - [_Stage 3: Create a `PVC` from Snapshot created at stage 2 and create a `POD` that use it_](#stage-3-create-a-pvc-from-snapshot-created-at-stage-2-and-create-a-pod-that-use-it)
      - [_Stage 4: Create a `PVC` from the `PVC` we created at stage 3 and create a `POD` that use it_](#stage-4-create-a-pvc-from-the-pvc-we-created-at-stage-3-and-create-a-pod-that-use-it)
      - [Uninstall Snapshot Workloads](#uninstall-snapshot-workloads)
    - [Install in different namespace](#install-in-different-namespace)
    - [Rendering Manifests Using Helm Chart](#rendering-manifests-using-helm-chart)

## Overview

We provide some workload deployment examples that use `lb-csi-plugin` for storage provisioning.

To ease the deployment of these workloads and to make them easily customizable we provide an Helm Chart
as part of the `lb-csi-bundle-<version>.tar.gz`.

This Helm Chart is comprized of six sub-chart. Each sub-chart defines a workload manifest.

All sub-charts are dependent on the storageclass chart.

This chart should be created first and deleted last.

Without this Chart no other chart can be deployed and all deployments will fail.

Workload examples included:

- storageclass
- block
- filesystem
- preprovisioned
- snaps
- statefulset

## Helm Chart Content

```bash
├── lb-csi-workload-examples
│   ├── charts
│   │   ├── block
│   │   │   ├── Chart.yaml
│   │   │   ├── templates
│   │   │   │   ├── example-block-pod.yaml
│   │   │   │   └── example-block-pvc.yaml
│   │   │   └── values.yaml
│   │   ├── filesystem
│   │   │   ├── Chart.yaml
│   │   │   ├── templates
│   │   │   │   ├── example-fs-pod.yaml
│   │   │   │   └── example-fs-pvc.yaml
│   │   │   └── values.yaml
│   │   ├── preprovisioned
│   │   │   ├── Chart.yaml
│   │   │   ├── templates
│   │   │   │   └── example-pre-provisioned-pv.yaml
│   │   │   └── values.yaml
│   │   ├── snaps
│   │   │   ├── Chart.yaml
│   │   │   ├── templates
│   │   │   │   ├── 01.example-pvc.yaml
│   │   │   │   ├── 02.example-pod.yaml
│   │   │   │   ├── 03.example-snapshot.yaml
│   │   │   │   ├── 04.example-pvc-from-snapshot.yaml
│   │   │   │   ├── 05.example-pvc-from-snapshot-pod.yaml
│   │   │   │   ├── 06.example-pvc-from-pvc.yaml
│   │   │   │   ├── 07.example-pvc-from-pvc-pod.yaml
│   │   │   │   ├── NOTES.txt
│   │   │   │   └── snapshot-sc.yaml
│   │   │   └── values.yaml
│   │   ├── statefulset
│   │   │   ├── Chart.yaml
│   │   │   ├── templates
│   │   │   │   └── example-sts.yaml
│   │   │   └── values.yaml
│   │   └── storageclass
│   │       ├── Chart.yaml
│   │       ├── lightos-jwt.toml
│   │       ├── templates
│   │       │   ├── secret.yaml
│   │       │   └── storageclass.yaml
│   │       └── values.yaml
│   ├── Chart.yaml
│   ├── README.md
│   ├── templates
│   └── values.yaml
```

### Chart Values

Workload examples are configurable using the [lb-csi-workload-examples/values.yaml](./lb-csi-workload-examples/values.yaml) file.

All workloads are disabled by default, and can be enabled by the `<workload_name>.enabled` property.

All examples share the same `StorageClass` and `Secret` templates.

To override values in these templates you can:

- Modify field in `values.yaml` file.
- Use the `--set` flag on helm install command.

Example provided `values.yaml` file:

```yaml
global:
  storageClass:
    name: example-sc
    # Name of the LightOS project we want the plugin to target.
    projectName: default
    # LightOS cluster API endpoints
    mgmtEndpoints: "" # required! comma delimited endpoints string, for example <ip>:<port>,<ip>:<port>
    # Number of replicas for each volume provisioned by this StorageClass
    replicaCount: "3"
    compression: disabled
    secretName: example-secret
    secretNamespace: default

# subchart workloads:
storageclass:
  enabled: false  
block:
  enabled: false  
filesystem:
  enabled: false
preprovisioned:
  enabled: false
  lightosVolNguid: "" # required! nguid of LightOS volume.
statefulset:
  enabled: false
  statefulSetName: example-sts
snaps:
  enabled: false
  pvcName: example-pvc
  stage: "" # required! one of ["example-pvc", "snapshot-from-pvc", "pvc-from-snapshot", "pvc-from-pvc"]
  snapshotStorageClass:
    name: example-snapshot-sc
```

Values Description:

| name   |  description  | default         | required   |
|--------|---------------|-----------------|------------|
| storageclass.enable   | Deploy Secret, StorageClass | false | false |
| block.enable          | Deploy block volume workload   | false | false |
| filesyste.enable      | Deploy filesystem volume workload   | false | false |
| statefulset.enable    | Deploy statefulset workload   | false | false |
| preprovisioned.enable | Deploy preprovisioned volume workload  | false | false |
| preprovisioned.lightosVolNguid | NGUID of LightOS volume   | ""  | false |
| snaps.enable  | Deploy Snapshot workloads   | false  | false |
| snaps.pvcName | Name of the pvc for Snapshot example |  example-pvc    | false |
| snaps.stage    | name the snapshot stage we want to execute | ""  | false |
| global.storageClass.mgmtEndpoints | LightOS API endpoint list, ex: `<ip>:<port>,...<ip>:<port>` | "" | true |
| global.storageClass.projectName | Created resoures will be scoped to this project | default | false |
| global.storageClass.replicaCount | Number of replicas for each volume | 3 | false |
| global.storageClass.compression | Rather copressions in enabled/disabled | disabled | false |
| global.storageClass.secretName | Secret containing `JWT` to authenticate against LightOS API | example-secret | true |
| global.storageClass.secretNamespace | Namespace the secret is defined at | default | true |

#### Mandatory Values To Modify

Following values **MUST** be modified to match target Kubernetes cluster.

- LightOS Cluster API Endpoints (`mgmt-endpoint`)

  Before we deploy a workload we to fetch some information from LightOS cluster

  `lb-cs-plugin` needs to be informed about LightOS management API endpoints.

  These endpoints are passed as a comma delimited string in `StorageClass.Parameters.mgmt-endpoints`.

  set `MGMT_EP` environment variable, by fetching `mgmtEndpoints` from `lbcli` by running following command:

  ```bash
  export MGMT_EP=$(lbcli get cluster -o json | jq -r '.apiEndpoints | join("\\,")')
  ```

  **NOTICE:** The '\\' in the join command. When passing this value to helm we must use the escape character `\`.

- LightOS API JWT

  Each API call to LightOS require a JWT for authentication and authorization.

  The JWT is passed to the plugin by creating a Kubernetes Secret resource.

  The Provided chart can be used to automate the process of creating this Secret by providing the JWT in `helm/lb-csi-workload-examples/charts/storageclass/lightos-jwt.toml` file.

  Helm will read the file and create the following `Secret`:

  ```yaml
  # Source: lb-csi-workload-examples/templates/secret.yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: example-secret
    namespace: default
  type: kubernetes.io/lb-csi
  data:
    jwt: |-
      ZXlKaGJHY2lPaUpTVXpJMU5pSXNJbXRwWkNJNkluTjVjM1JsYlRweWIyOTBJaXdpZEhsd0lqb2lTbGRVSW4wLmV5SmhkV1FpT2lKTWFXZG9kRTlUSWl3aVpYaHdJam94TmpRMU5ESXdORE15TENKcFlYUWlPakUyTVRNNE9EUTBNeklzSW1semN5STZJbk41YzNSbGMzUnpJaXdpYW5ScElqb2lWRXh5VHpoSWVrTjNiek5qTlV4UlJuazVTV3BvVVNJc0ltNWlaaUk2TVRZeE16ZzRORFF6TWl3aWNtOXNaWE1pT2xzaWMzbHpkR1Z0T21Oc2RYTjBaWEl0WVdSdGFXNGlYU3dpYzNWaUlqb2liR2xuYUhSdmN5MWpiR2xsYm5RaWZRLkpBNExwcWExRzFzZGZ3bE1zRVBWNzZCbE1uZVA1bnFzdlZOTzQ2N0l3MUNHSzFjVUNZLWk5MGpjVmdTM1YxVmlCN3J1MG5mX2JkaEdvX091WERaaHktQzVXeGVocVVtaFk0V3NhdWlHejNnQ2NHc3Roa21TbHVkNUlXeXZ4djM5ZEJPenJ0MGJDVW9ELXdVSEdUeC14eUpLWVc0MjFSM19sRW1TTm1KeDRHZUc4NV9GQkNiSU93OGF2YUl5eDJlNXFBeDBpTTdhSDZCTlo0S2tiQ0tnZmtjVl9MRDBqQUtfWUVyeThGdi1NRDU4cGVrZXVNQ0dkWTdfWVBPdG5KelIweUZ2dG9PZmNOdnAxLXRXNXNDbkUwWTliUV9FX3lzMlVYMjlia25OUTJhYmRoeU5FN0ZjeWk3QlZtVnNWYTBfUzhQMU9OaXZHODNQOVYybUdPd1czQQo=
  ```

## Usage

Ideally, the output should contain no error messages. If you see any, try to determine if the problem is with the connectivity to the Kubernetes cluster, the kubelet configuration, or some other minor issue.

After the above command completes, the deployment process can take between several seconds and several minutes, depending on the size of the Kubernetes cluster, load on the cluster nodes, network connectivity, etc.

After a short while, you can issue the following commands to verify the results. Your output will likely differ from the following example, including to reflect your Kubernetes cluster configuration, randomly generated pod names, etc.

### Secret, StorageClass and VolumeSnapshotClass Chart

This Chart will install the following resources:

- A `Secret` containing the lightos JWT
- A `StorageClass` referenceing the secret and configured with all values needed to provision volumes on LightOS.
- A `SnapshotStorageClass` referenceing the secret.

#### Deploy Secret And StorageClasses Workload

```bash
helm install \
  --set storageclass.enabled=true \
  --set global.storageClass.mgmtEndpoints="$MGMT_EP" lb-csi-workload-examples-sc \
  helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-examples-sc
LAST DEPLOYED: Sun Feb 21 16:12:56 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

#### Verify Secret And StorageClasses Workload

Verify that all resources where created:

```bash
kubectl get sc,secret,VolumeSnapshotClass
NAME                                     PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
storageclass.storage.k8s.io/example-sc   csi.lightbitslabs.com   Delete          Immediate           true                   5m27s

NAME                                                       TYPE                                  DATA   AGE
secret/example-secret                                      kubernetes.io/lb-csi                  1      5m27s

NAME                                                              DRIVER                  DELETIONPOLICY   AGE
volumesnapshotclass.snapshot.storage.k8s.io/example-snapshot-sc   csi.lightbitslabs.com   Delete           5m27s
```

#### Uninstall Secret And StorageClasses Workload

Once done with deployment examples you can delete storageclass resources.

```bash
helm uninstall lb-csi-workload-sc
release "lb-csi-workload-sc" uninstalled
```

### Deploy Block PVC and POD

This Chart will install the following resources:

- A `PVC` named `example-block-pvc` referencing previous defined `StorageClass`
- A `POD` named `example-block-pod` using `example-block-pvc`  

#### Deploy Block Workload

```bash
helm install \
  --set block.enabled=true \
  lb-csi-workload-block \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-block
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

#### Verify Block Workload

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state.

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                       STORAGECLASS   REASON   AGE
persistentvolume/pvc-2b3b510d-bc4c-4528-a431-3923b8b7d443   3Gi        RWO            Delete           Bound    default/example-block-pvc   example-sc              2m55s

NAME                                      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-block-pvc   Bound    pvc-2b3b510d-bc4c-4528-a431-3923b8b7d443   3Gi        RWO            example-sc     2m56s

NAME                    READY   STATUS    RESTARTS   AGE
pod/example-block-pod   1/1     Running   0          2m56s
```

#### Uninstall Block Workload

```bash
helm uninstall lb-csi-workload-block
```

Verify all resources are gone

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

### Filesystem PVC and POD Workload

This Chart will install the following resources:

- A `PVC` named `example-fs-pvc` referencing previous defined `StorageClass`
- A `POD` named `example-fs-pod` using `example-fs-pvc`  

#### Deploy Filesystem Workload

```bash
helm install \
  --set filesystem.enabled=true \
  lb-csi-workload-filesystem \
  ./helm/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-filesystem
LAST DEPLOYED: Sun Feb 21 16:09:02 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

#### Verify Filesystem Workload Deployed

Verify that `PV`, `PVC` created and in `Bounded` state and `POD` is in `Running` state:

```bash
kubectl get pv,pvc,pods
NAME                                                        CAPACITY   ACCESS MODES   RECLAIM POLICY   STATUS   CLAIM                    STORAGECLASS   REASON   AGE
persistentvolume/pvc-e0ad4f63-4b42-417f-8bed-94f8aec8f0d5   10Gi       RWO            Delete           Bound    default/example-fs-pvc   example-sc              33s

NAME                                   STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
persistentvolumeclaim/example-fs-pvc   Bound    pvc-e0ad4f63-4b42-417f-8bed-94f8aec8f0d5   10Gi       RWO            example-sc     34s

NAME                 READY   STATUS    RESTARTS   AGE
pod/example-fs-pod   1/1     Running   0          34s
```

#### Uninstall Filesystem Workload

```bash
helm uninstall lb-csi-workload-filesystem
```

Verify all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

### Deploy StatefulSet

#### Deploy StatefulSet Workload

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

#### Verify StatefulSet Workload

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

#### Uninstall StatefulSet Workload

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

### Deploy Snapshot and Clones Workloads

This examples is a bit more complex and is build of six different stages:

- [Stage 1: Create `VolumeSnapshotClass`](#stage-1-create-volumesnapshotclass)
- [Stage 2: Create Example `PVC` and `POD`](#stage-2-create-example-pvc-and-pod)
- [Stage 3: Take a `Snapshot` from PVC created at stage 2](#stage-3-take-a-snapshot-from-pvc-created-at-stage-2)
- [Stage 4: Create a `PVC` from Snapshot created at stage 3 and create a `POD` that use it](#stage-4-create-a-pvc-from-snapshot-created-at-stage-3-and-create-a-pod-that-use-it)
- [Stage 5: Create a `PVC` from the `PVC` we created at stage 4 and create a `POD` that use it](#stage-5-create-a-pvc-from-the-pvc-we-created-at-stage-3-and-create-a-pod-that-use-it)
- [Stage 6: Uninstall Snapshot Workloads](#stage-6-uninstall-snapshot-workloads)

The examples are dependent on one another, so you must to run them in order.

For Helm to deploy the `snaps` Chart in stages we introduce the mandatory variable `snaps.stage`
The Chart support four stages:

- snapshot-class
- example-pvc
- snapshot-from-pvc
- pvc-from-snapshot
- pvc-from-pvc

The examples are dependent on one another, so you must run them in order.

#### _Stage 1: Create `VolumeSnapshotClass`_

Create a `VolumeSnapshotClass`:

```bash
helm install \
  --set snaps.enabled=true \
  --set snaps.stage=snapshot-class \
  lb-csi-workload-snaps-snapshot-class \
  ./helm/lb-csi-workload-examples
```

#### _Stage 2: Create Example `PVC` and `POD`_

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

#### _Stage 3: Take a `Snapshot` from PVC created at stage 2_

Create a snapshot from previously created `PVC` named `example-pvc`

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

Verify `VolumeSnapshot` and `VolumeSnapshotContent` were created, and `READYTOUSE` status is `true`

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent

NAME                                                      READYTOUSE   SOURCEPVC     SOURCESNAPSHOTCONTENT   RESTORESIZE   SNAPSHOTCLASS         SNAPSHOTCONTENT                                    CREATIONTIME   AGE
volumesnapshot.snapshot.storage.k8s.io/example-snapshot   true         example-pvc                           10Gi          example-snapshot-sc   snapcontent-b710e398-eaa5-45be-bbdc-db74d799e5cc   3m40s          3m49s

NAME                                                                                             READYTOUSE   RESTORESIZE   DELETIONPOLICY   DRIVER                  VOLUMESNAPSHOTCLASS   VOLUMESNAPSHOT     AGE
volumesnapshotcontent.snapshot.storage.k8s.io/snapcontent-b710e398-eaa5-45be-bbdc-db74d799e5cc   true         10737418240   Delete           csi.lightbitslabs.com   example-snapshot-sc   example-snapshot   3m49s
```

#### _Stage 4: Create a `PVC` from Snapshot created at stage 3 and create a `POD` that use it_

Create a `PVC` from previously taken `Snapshot` named `example-snapshot`

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

#### _Stage 5: Create a `PVC` from the `PVC` we created at stage 3 and create a `POD` that use it_

Create a `PVC` from previously taken `Snapshot` named `example-snapshot`

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

#### _Stage 6: Uninstall Snapshot Workloads_

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

After each uninstall we need to verify that all related resources were released before continue to next uninstall.

Uninstall `pvc-from-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-pvc-from-pvc
```

In order to verify that all resources are deleted the following command should return no entry:

```bash
kubectl get pv,pvc,pod | grep pvc-from-pvc
```

Uninstall `pvc-from-snapshot`:

```bash
helm uninstall lb-csi-workload-snaps-pvc-from-snapshot
```

In order to verify that all resources are deleted the following command should return no entry:

```bash
kubectl get pv,pvc,pod | grep pvc-from-snapshot
```

Uninstall `snapshot-from-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-snapshot-from-pvc
```

In order to verify that all resources are deleted the following command should return no entry:

```bash
kubectl get VolumeSnapshot,VolumeSnapshotContent | grep snapshot-from-pvc
```

Uninstall `example-pvc`:

```bash
helm uninstall lb-csi-workload-snaps-example-pvc
```

Verify all resources are gone:

```bash
kubectl get pv,pvc,pods
No resources found in default namespace.
```

Delete `VolumeSnapshotClass`:

```bash
helm uninstall lb-csi-workload-snaps-snapshot-class
```


### Install in different namespace

You can install the workloads in a different namespace (ex: `lb-csi-ns`)
by creating a namespace your self or using the shortcut to let helm create a namespace for you:

```bash
helm install -n lb-csi-ns --create-namespace \
	lb-csi-workload-examples \
  helm/lb-csi-workload-examples/
```

### Rendering Manifests Using Helm Chart

Render manifests to file `/tmp/filesystem-workload.yaml` run following command:

```bash
helm template \
	--set filesystem.enabled=true \
	lb-csi-workload-examples > /tmp/filesystem-workload.yaml
```

The chart enable to render multiple workload in the same time using the following command:

```bash
helm template \
	--set block.enabled=true \
	--set filesystem.enabled=true \
	lb-csi-workload-examples > /tmp/block-and-filesystem-workload.yaml
```

Outcome is placed at `/tmp/block-and-filesystem-workload.yaml`.
