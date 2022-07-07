## Secret and StorageClass

> **NOTE:**
>
> For these workloads to work, you are required at a minimum to provide the following cluster specific parameters:
> - `JWT`
> - `mgmt-endpoints`
>
> Without modifying these parameters, the workloads will likely fail.

### Storing LightOS Authentication JWT in a Kubernetes Secret

Kubernetes and the CSI specification support passing "secrets" when invoking most of the CSI plugin operations, which the plugins can use for authentication and authorization against the corresponding storage provider. This allows a single CSI plugin deployment to serve multiple unrelated users or tenants and to authenticate with the storage provider on their behalf using their credentials, as necessary. The Lightbits CSI plugin takes advantage of this "secrets" passing functionality for authentication and authorization.

> **Note:**
>
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

In the file `examples/secret-and-storage-class.yaml`, edit the `Secret.data.jwt` value with the base64 encoded `JWT` string.

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
  qos-policy-name: <qos-policy name>
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

| Placeholder                     | Description                                                                               |
|---------------------------------|-------------------------------------------------------------------------------------------|
| `<sc-name>`              | The name of the StorageClass you want to define. This name will be referenced from other Kubernetes object specs (e.g.: StatefulSet, PersistentVolumeClaim) to use a volume that will be provisioned from a LightOS storage cluster mentioned below, with the corresponding volume characteristics.|
| `<true\|false>`<br>(allowVolumeExpansion) | Kubernetes PersistentVolume-s can be configured to be expandable and LightOS supports volume expansion. If set to true, it will be possible to expand the volumes created from this StorageClass by editing the corresponding PVC Kubernetes objects.<br>**Note:**<br>CSI volumes expansion is enabled in Kubernetes v1.16 and above. CSI volume expansion in older Kubernetes versions is not supported by the Lightbits CSI plugin.|
| `<lb-mgmt-address>`     | One of the LightOS management API service endpoint IP addresses of the LightOS cluster on which the volumes belonging to this StorageClass will be created.<br>The mgmt-endpoint entry of the StorageClass spec accepts a comma-separated list of `<lb-mgmt-address>:<lb-mgmt-port>` pairs.<br>For high availability, specify the management API service endpoints of all the LightOS cluster servers, or at least the majority of the servers.|
| `<lb-mgmt-port>`        | The port number on which the LightOS management API service is running. Typically, this is port 443 and port 80 for encrypted and encrypted communications, respectively - but LightOS servers can be configured to serve the management interface on other ports as well.|
| `<grpc\|grpcs>`         | The protocol to use for communication with the LightOS management API service. LightOS clusters with multi-tenancy support enabled can be accessed only over the TLS-protected grpcs protocol for enhanced security. LightOS clusters with multi-tenancy support disabled can be accessed using the legacy unencrypted grpc protocol.|
| `<proj-name>`           | The name of the LightOS project to which the volumes from this StorageClass will belong. The JWT specified using `<secret-name>` below must have sufficient permissions to carry out the necessary actions in that project. |
| `<num-replicas>`        | The desired number of replicas for volumes dynamically provisioned for this StorageClass. Valid values are: 1, 2 or 3. The number must be specified in ASCII double quotes (e.g.: "2").|
| `<enabled\|disabled>`   | Specifies whether the volumes created for this StorageClass should have compression enabled or disabled. The compression line of the StorageClass spec can be omitted altogether, in which case the LightOS storage cluster default setting for compression will be used. However, if it is present, it must contain one of the following two values: enabled or disabled.|
| `<secret-name>`         | The name of the Kubernetes Secret that holds the JWT to be used while making requests pertaining to this StorageClass to the LightOS management API service. See also `<secret-namespace>` below.<br>Typically the JWT used for all the different types of operations (5 in the examples below) will be the same JWT, but there is no requirement for that to be the case.|
| `<secret-namespace>`    | The namespace in which the Secret referred to in `<secret-name>` above resides.|
| `<qos-policy-name>`     | New volumes created will be attached with that qos policy. Default value is "" which means using the default qos profile|

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
  qos-policy-name: "example-pol"
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
secret/example-secret        lightbitslabs.com/jwt                 1      59s

NAME                                     PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
storageclass.storage.k8s.io/example-sc   csi.lightbitslabs.com   Delete          Immediate           true                   59s
```

> **NOTE:**
>
> You can create as many StorageClass-es as you need based on a single or multiple LightOS storage clusters and with different replication factor and compression settings, belonging to the same or different LightOS projects.
