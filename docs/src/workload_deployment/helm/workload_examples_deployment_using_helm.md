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

To ease the deployment of these workloads and to make them easily customizable we provide a Helm Chart as part of the `lb-csi-bundle-<version>.tar.gz`.

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
│   │       ├── templates
│   │       │   ├── secret.yaml
│   │       │   └── storageclass.yaml
│   │       └── values.yaml
│   ├── Chart.yaml
│   ├── README.md
│   ├── templates
│   └── values.yaml
```

#### Chart Values

Workload examples are configurable using the [lb-csi-workload-examples/values.yaml](./lb-csi-workload-examples/values.yaml) file.

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
    secretName: example-secret
    secretNamespace: default

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
  nodeName: ""
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

| name                                   |  description                                                       | default | required   |
|----------------------------------------|--------------------------------------------------------------------|---------|------------|
| storageclass.enable                    | Deploy Secret, StorageClass                                        | false   | false |
| block.enable                           | Deploy block volume workload                                       | false   | false |
| block.nodeSelector                     | Deploy `POD` on specific node using node selectors                 | {}      | false |
| block.nodeName                         | Deploy `POD` on specific node using node name                      | ""      | false |
| filesystem.enable                      | Deploy filesystem volume workload                                  | false   | false |
| filesystem.nodeSelector                | Deploy `POD` on specific node using node selectors                 | {}      | false |
| filesystem.nodeName                    | Deploy `POD` on specific node using node name                      | ""      | false |
| statefulset.enable                     | Deploy statefulset workload                                        | false   | false |
| preprovisioned.enable                  | Deploy preprovisioned volume workload                              | false   | false |
| preprovisioned.lightosVolNguid         | NGUID of LightOS volume                                            | ""      | false |
| snaps.enable                           | Deploy snapshot workloads                                          | false   | false |
| snaps.pvcName                          | Name of the pvc for snapshot example                               | example-pvc | false |
| snaps.stage                            | Name the snapshot stage we want to execute                         | ""  | false |
| global.storageClass.mgmtEndpoints      | LightOS API endpoint list, ex: `<ip>:<port>,...<ip>:<port>`        | "" | true |
| global.storageClass.projectName        | Created resources will be scoped to this project                   | default | false |
| global.storageClass.replicaCount       | Number of replicas for each volume                                 | 3 | false |
| global.storageClass.compression        | Rather compressions in enabled/disabled                            | disabled | false |
| global.jwtSecret.name                  | Secret name that holds LightOS API `JWT`                           | example-secret | true |
| global.jwtSecret.namespace             | Namespace the secret is defined at                                 | default | true |
| global.jwtSecret.jwt                   | `JWT` to authenticate against LightOS API                          | default | true |

##### Mandatory Values To Modify

The following values **MUST** be modified to match the target Kubernetes cluster.

- LightOS Cluster API Endpoints (`mgmt-endpoint`)

  Before we deploy a workload we need to fetch some information from the LightOS cluster.

  `lb-csi-plugin` needs to be informed about LightOS management API endpoints.

  These endpoints are passed as a comma-delimited string in `StorageClass.Parameters.mgmt-endpoints`.

  set `MGMT_EP` environment variable, by fetching `mgmtEndpoints` from `lbcli` by running the following command:

  ```bash
  export MGMT_EP=$(lbcli get cluster -o json | jq -r '.apiEndpoints | join("\\,")')
  ```

  > **NOTE:** 
  > 
  > The '\\' in the join command. When passing this value to Helm we must use the escape character `\`.

- LightOS API JWT

  Each API call to LightOS requires a JWT for authentication and authorization.

  The JWT is passed to the plugin by creating a Kubernetes Secret resource.

  Set the `LIGHTOS_JWT` environment variable, by fetching `mgmtEndpoints` from `lbcli` by running the following command:

  ```bash
  export LIGHTOS_JWT=eyJhbGciOiJSUzI1NiIsIm...ie0yJR0ZlMppc8U4F-KQ
  ```

  > **NOTICE:** 
  > 
  > K8S stores the secret data base64 encoded but the chart will do
  > the encoding for you. 

  Helm will generate a `Secret` looking like:

  ```yaml
  # Source: lb-csi-workload-examples/templates/secret.yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: example-secret
    namespace: default
  type: lightbitslabs.com/jwt
  data:
    jwt: |-
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

#### Install in different namespace

You can install the workloads in a different namespace (ex: `lb-csi-ns`)
by creating a namespace yourself or using the shortcut to let Helm create a namespace for you:

```bash
helm install -n lb-csi-ns --create-namespace \
	lb-csi-workload-examples \
  ./helm/lb-csi-workload-examples
```

#### Rendering Manifests Using the Helm Chart

Render manifests to file `/tmp/filesystem-workload.yaml` by running the following command:

```bash
helm template \
	--set filesystem.enabled=true \
	./helm/lb-csi-workload-examples > /tmp/filesystem-workload.yaml
```

The chart enables rendering multiple workloads at the same time using the following command:

```bash
helm template \
	--set block.enabled=true \
	--set filesystem.enabled=true \
	./helm/lb-csi-workload-examples > /tmp/block-and-filesystem-workload.yaml
```

Outcome is placed at `/tmp/block-and-filesystem-workload.yaml`.
