<div style="page-break-after: always;"></div>

## Workload Examples Deployment Using Helm

The Helm chart eases the deployment of the provided workload examples that use the `lb-csi-plugin` as a persistent storage backend.

- [Workload Examples Deployment Using Helm](#workload-examples-deployment-using-helm)
  - [Overview](#overview)
  - [Helm Chart Content](#helm-chart-content)
    - [Chart Values](#chart-values)
      - [Mandatory Values To Modify](#mandatory-values-to-modify)
    - [Install in different namespace](#install-in-different-namespace)
    - [Rendering Manifests Using the Helm Chart](#rendering-manifests-using-the-helm-chart)

### Overview

We provide some workload deployment examples that use `lb-csi-plugin` for storage provisioning.

To ease the deployment of these workloads and to make them easily customizable we provide the `lb-csi-workload-examples` Helm Chart. It is available from the [Lightbits Helm Repository](../../plugin_deployment/plugin_deployment_using_lightbits_helm_repository.md) as `lightbits-helm-repo/lb-csi-workload-examples`, and inside the `lb-csi-bundle-<version>.tar.gz` as `helm/charts/lb-csi-workload-examples-<chart-version>.tgz`. The commands in this section use the repository chart. To use the bundled chart, replace `lightbits-helm-repo/lb-csi-workload-examples` with the path of the `.tgz` file.

This Helm Chart is comprised of six sub-charts. Each sub-chart defines a set of manifests representing a workload.

All sub-charts are dependent on the StorageClass chart. All following PVC created by the other charts will use the StorageClass we create using the StorageClass Chart. Hence, we should first install this chart and uninstall only after we uninstalled all other charts.

Workload examples include:

- StorageClass
- Block
- Filesystem
- Pre-provisioned volume
- Snapshots and Clones
- StatefulSet

### Helm Chart Content

```bash
lb-csi-workload-examples
├── Chart.yaml
├── charts
│   ├── block
│   │   ├── Chart.yaml
│   │   ├── templates
│   │   │   ├── example-block-pod.yaml
│   │   │   └── example-block-pvc.yaml
│   │   └── values.yaml
│   ├── filesystem
│   │   ├── Chart.yaml
│   │   ├── templates
│   │   │   ├── example-fs-pod.yaml
│   │   │   └── example-fs-pvc.yaml
│   │   └── values.yaml
│   ├── preprovisioned
│   │   ├── Chart.yaml
│   │   ├── templates
│   │   │   ├── example-pre-provisioned-pod.yaml
│   │   │   ├── example-pre-provisioned-pv.yaml
│   │   │   └── example-pre-provisioned-pvc.yaml
│   │   └── values.yaml
│   ├── snaps
│   │   ├── Chart.yaml
│   │   ├── templates
│   │   │   ├── 00.snapshot-sc.yaml
│   │   │   ├── 01.example-pvc.yaml
│   │   │   ├── 02.example-pod.yaml
│   │   │   ├── 03.example-snapshot.yaml
│   │   │   ├── 04.example-pvc-from-snapshot.yaml
│   │   │   ├── 05.example-pvc-from-snapshot-pod.yaml
│   │   │   ├── 06.example-pvc-from-pvc.yaml
│   │   │   ├── 07.example-pvc-from-pvc-pod.yaml
│   │   │   └── NOTES.txt
│   │   └── values.yaml
│   ├── statefulset
│   │   ├── Chart.yaml
│   │   ├── templates
│   │   │   └── example-sts.yaml
│   │   └── values.yaml
│   └── storageclass
│       ├── Chart.yaml
│       ├── templates
│       │   ├── secret.yaml
│       │   └── storageclass.yaml
│       └── values.yaml
├── templates
│   └── NOTES.txt
├── values.schema.json
└── values.yaml
```

#### Chart Values

Workload examples are configurable using the chart's `values.yaml` file. To print it, run:

```bash
helm show values lightbits-helm-repo/lb-csi-workload-examples
```

All workloads are disabled by default, and can be enabled by the `<workload_name>.enabled` property.

All examples share the same `StorageClass` and `Secret` templates.

To override values in these templates you can:

- Modify fields in the `values.yaml` file.
- Use the `--set` flag on the helm install command.

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
    qosPolicyName: ""
    host-encryption: disabled
    # Optional. When "enabled", the node plugin restricts each volume's IP-ACL
    # to the data-path addresses of the nodes it is staged on.
    ipAcl: disabled
    # The csi.storage.k8s.io/fstype parameter is optional. The values allowed are ext4 or xfs. The default value is ext4.
    fsType: "ext4"
  jwtSecret:
    name: example-secret
    namespace: default
    jwt: "" # required!

# subchart workloads:
storageclass:
  enabled: false
block:
  enabled: false
  nodeSelector: {}
  nodeName: ""
filesystem:
  enabled: false
  nodeSelector: {}
  nodeName: "" # optional node name this pod will be deployed on.
preprovisioned:
  enabled: false
  lightosVolNguid: "" # required! nguid of LightOS volume.
  volumeMode: "" # required! one of ["Block", "Filesystem"]
  storage: "" # required! 1Gi
statefulset:
  enabled: false
  statefulSetName: example-sts
snaps:
  enabled: false
  pvcName: example-pvc
  stage: "" # required! one of ["example-pvc", "snapshot-from-pvc", "pvc-from-snapshot", "pvc-from-pvc", "snapshot-class", ""]
  kubeVersion: ""
  snapshotStorageClass:
    name: example-snapshot-sc
```

Values Description:

| name                               | description                                                        | default        | required |
| ---------------------------------- | ------------------------------------------------------------------ | -------------- | -------- |
| storageclass.enabled               | Deploy Secret, StorageClass                                        | false          | false    |
| block.enabled                      | Deploy block volume workload                                       | false          | false    |
| block.nodeSelector                 | Deploy `POD` on specific node using node selectors                 | {}             | false    |
| block.nodeName                     | Deploy `POD` on specific node using node name                      | ""             | false    |
| filesystem.enabled                 | Deploy filesystem volume workload                                  | false          | false    |
| filesystem.nodeSelector            | Deploy `POD` on specific node using node selectors                 | {}             | false    |
| filesystem.nodeName                | Deploy `POD` on specific node using node name                      | ""             | false    |
| statefulset.enabled                | Deploy statefulset workload                                        | false          | false    |
| preprovisioned.enabled             | Deploy preprovisioned volume workload                              | false          | false    |
| preprovisioned.lightosVolNguid     | NGUID of LightOS volume                                            | ""             | true     |
| preprovisioned.volumeMode          | Specifies if this volume is used for Block or Filesystem           | ""             | true     |
| preprovisioned.storage             | Size of the volume this PV will point to.                          | ""             | true     |
| snaps.enabled                      | Deploy snapshot workloads                                          | false          | false    |
| snaps.pvcName                      | Name of the pvc for snapshot example                               | example-pvc    | false    |
| snaps.stage                        | Name the snapshot stage we want to execute                         | ""             | false    |
| global.storageClass.name           | Name of the StorageClass                                           | example-sc     | false    |
| global.storageClass.mgmtEndpoints  | LightOS API endpoint list, ex: `<ip>:<port>,...<ip>:<port>`        | ""             | true     |
| global.storageClass.projectName    | Created resources will be scoped to this project                   | default        | false    |
| global.storageClass.replicaCount   | Number of replicas for each volume                                 | 3              | false    |
| global.storageClass.compression    | Whether compressions in enabled/disabled                           | disabled       | false    |
| global.storageClass.qosPolicyName  | qos policy name, should exist in the Lightos prior volume creation | ""             | false    |
| global.storageClass.hostEncryption | Whether host-side encryption is enabled/disabled                   | disabled       | false    |
| global.storageClass.ipAcl          | Whether volumes are IP-ACL-restricted to their nodes' addresses    | disabled       | false    |
| global.storageClass.fsType         | File system of filesystem volumes: `ext4` or `xfs`                 | ext4           | false    |
| global.jwtSecret.name              | Secret name that holds LightOS API `JWT`                           | example-secret | false    |
| global.jwtSecret.namespace         | Namespace the secret is defined at                                 | default        | false    |
| global.jwtSecret.jwt               | `JWT` to authenticate against LightOS API                          | ""             | true     |

##### Mandatory Values To Modify

The following values **MUST** be modified to match the target Kubernetes cluster.

- LightOS Cluster API Endpoints (`mgmt-endpoint`)

  Before we deploy a workload we need to fetch some information from the LightOS cluster.

  `lb-csi-plugin` needs to be informed about LightOS management API endpoints.

  These endpoints are passed as a comma-delimited string in `StorageClass.Parameters.mgmt-endpoints`.

  Set the `MGMT_EP` environment variable, by fetching `apiEndpoints` from `lbcli` by running the following command:

  ```bash
  export MGMT_EP=$(lbcli get cluster -o json | jq -r '.apiEndpoints | join("\\,")')
  ```

  > **NOTE:**
  >
  > The join command puts `\` before each comma. Helm `--set` reads an unescaped comma as the start of a new value, so the commas must be escaped.

- LightOS API JWT

  Each API call to LightOS requires a JWT for authentication and authorization.

  The JWT is passed to the plugin by creating a Kubernetes Secret resource.

  Set the `LIGHTOS_JWT` environment variable to the JWT:

  ```bash
  export LIGHTOS_JWT=eyJhbGciOiJSUzI1NiIsIm...ie0yJR0ZlMppc8U4F-KQ
  ```

  > **NOTICE:**
  >
  > K8S stores the secret data base64 encoded but the chart will do
  > the encoding for you.

Pass both values when you install the `storageclass` workload:

```bash
--set global.storageClass.mgmtEndpoints="$MGMT_EP" \
--set global.jwtSecret.jwt="$LIGHTOS_JWT"
```

#### Install in different namespace

You can install the workloads in a different namespace (ex: `lb-csi-ns`)
by creating a namespace yourself or using the shortcut to let Helm create a namespace for you.

The `Secret` is created in `global.jwtSecret.namespace`, not in the release namespace. Set it to the same namespace:

```bash
helm install \
  -n lb-csi-ns \
  --create-namespace \
  --set storageclass.enabled=true \
  --set global.storageClass.mgmtEndpoints="$MGMT_EP" \
  --set global.jwtSecret.jwt="$LIGHTOS_JWT" \
  --set global.jwtSecret.namespace=lb-csi-ns \
  lb-csi-workload-examples-sc \
  lightbits-helm-repo/lb-csi-workload-examples
```

#### Rendering Manifests Using the Helm Chart

Render manifests to file `/tmp/filesystem-workload.yaml` by running the following command:

```bash
helm template \
  --set filesystem.enabled=true \
  lightbits-helm-repo/lb-csi-workload-examples > /tmp/filesystem-workload.yaml
```

The chart enables rendering multiple workloads at the same time using the following command:

```bash
helm template \
  --set block.enabled=true \
  --set filesystem.enabled=true \
  lightbits-helm-repo/lb-csi-workload-examples > /tmp/block-and-filesystem-workload.yaml
```

Outcome is placed at `/tmp/block-and-filesystem-workload.yaml`.
