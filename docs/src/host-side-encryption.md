<div style="page-break-after: always;"></div>
\pagebreak

# Encryption

Encryption of volumes is possible in two different ways, either on the Lightbits side, or on the host side.

## host side encryption

With this method, the consumer of a volume defines a secret which is used to encrypt the volume content before it is sent over to Lightbits.
This is forced by some customers to meet very high security requirements, for example for health or military applications.

Downside is that with host side encryption server side compression should be disabled, because this will have no effect at all.
It is still possible to enable compression, but will probably hurt overall performance.

### configuration

To get a volume encrypted a secret must be provided and a storageclass, which enables encryption, must be created.
The secret can be given globally in the kube-system namespace, or on a per namespace basis.

Host side encryption does currently not support secrets on a per volume basis.

In the simplest case, one encryption secret in the kube-system namespace, the configuration would like like so:

```yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "false"
  creationTimestamp: "2022-01-24T08:40:03Z"
  name: encrypted-sc
parameters:
  compression: disabled
  host-encryption: enabled
  csi.storage.k8s.io/controller-expand-secret-name: lb-csi-creds
  csi.storage.k8s.io/controller-expand-secret-namespace: kube-system
  csi.storage.k8s.io/controller-publish-secret-name: lb-csi-creds
  csi.storage.k8s.io/controller-publish-secret-namespace: kube-system
  csi.storage.k8s.io/node-publish-secret-name: lb-csi-creds
  csi.storage.k8s.io/node-publish-secret-namespace: kube-system
  csi.storage.k8s.io/node-stage-secret-name: lb-csi-creds
  csi.storage.k8s.io/node-stage-secret-namespace: kube-system
  csi.storage.k8s.io/provisioner-secret-name: lb-csi-creds
  csi.storage.k8s.io/provisioner-secret-namespace: kube-system
  mgmt-endpoint: 10.131.44.1:443,10.131.44.2:443,10.131.44.3:443
  mgmt-scheme: grpcs
  project-name: 0f89286d-0429-4209-a8a9-8612befbff97
  replica-count: "3"
provisioner: csi.lightbitslabs.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

The Secret will then look like:

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: lb-csi-creds
  namespace: kube-system
type: Opaque
data:
  host-encryption-passphrase: bXlhd2Vzb21lcGFzc3BocmFzZQ==
  jwt: <the JWT token to authenticate against Lightbits>
```

The name of the key for encryption must be: `host-encryption-passphrase`.

If a finer grained secret handling is required, the CSI spec allows templating of params in the storageclass, with this something like this is possible:

```yaml
allowVolumeExpansion: true
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  annotations:
    storageclass.kubernetes.io/is-default-class: "false"
  creationTimestamp: "2022-01-24T08:40:03Z"
  name: partition-gold-encrypted
parameters:
  compression: disabled
  host-encryption: enabled
  encryption-secret-namespace: ${pvc.namespace}
  encryption-secret-name: storage-encryption-key
  csi.storage.k8s.io/controller-expand-secret-name: lb-csi-creds
  csi.storage.k8s.io/controller-expand-secret-namespace: kube-system
  csi.storage.k8s.io/controller-publish-secret-name: lb-csi-creds
  csi.storage.k8s.io/controller-publish-secret-namespace: kube-system
  csi.storage.k8s.io/node-publish-secret-name: storage-encryption-key
  csi.storage.k8s.io/node-publish-secret-namespace: ${pvc.namespace}
  csi.storage.k8s.io/node-stage-secret-name: storage-encryption-key
  csi.storage.k8s.io/node-stage-secret-namespace: ${pvc.namespace}
  csi.storage.k8s.io/provisioner-secret-name: lb-csi-creds
  csi.storage.k8s.io/provisioner-secret-namespace: kube-system
  mgmt-endpoint: 10.131.44.1:443,10.131.44.2:443,10.131.44.3:443
  mgmt-scheme: grpcs
  project-name: 0f89286d-0429-4209-a8a9-8612befbff97
  replica-count: "3"
provisioner: csi.lightbitslabs.com
reclaimPolicy: Delete
volumeBindingMode: Immediate
```

Now, a storage encryption secret called `storage-encryption-key` must be present in the namespace of the PVC. This must also contain the `host-encryption-passphrase` as shown above.

Further explanation and samples can be found on the official [CSI documentation](https://kubernetes-csi.github.io/docs/secrets-and-credentials-storage-class.html#per-volume-secrets).

#### Custom LUKS Configuration

Host side encryption is done using LUKS disk encryption.

LUKS has many configuration parameters on format/open that may vary from one deployment to another.

The CSI plugin offers sane defaults for these parameters, and additionally it offers a way to override these settings on a per node basis.

Example to such a parameter override is `pbkdf-memory` which limits the amount of memory used to create the encrypted device according to [issues/372](https://gitlab.com/cryptsetup/cryptsetup/-/issues/372).

Since in a single cluster we may see variance in node resources with regards to memory constraints we need to limit this parameter to enable the same volume to open on all nodes.

The plugin will try to load the file placed at `/etc/lb-csi/luks_config.yaml` by default and will try to read this value from the file. if not present the plugin will use the default (64MB).

Plugin configuration required:

```yaml
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: lb-csi-node
  namespace: {{ .Release.Namespace }}
spec:
  selector:
    matchLabels:
      app: lb-csi-plugin
      role: node
    spec:
      containers:
      - name: lb-csi-plugin
        [...]
        env:
        - name: LB_CSI_LUKS_CONFIG_PATH
          value: "/etc/lb-csi-luks-config"
        volumeMounts:
        [...]
        - name: luks-config-dir
          mountPath: "/etc/lb-csi-luks-config"
      volumes:
      [...]
      - name: luks-config-dir
        hostPath:
          path: "/etc/lb-csi-luks-config"
          type: DirectoryOrCreate
```

Using `Helm` you can provide the following configuration value

```yaml
luksConfigDir: /etc/lb-csi-luks-config
```

In order to instruct the plugin to load the configuration.

An example of such `/etc/lb-csi-luks-config/luks_config.yaml` file would look like:

```bash
pbkdfMemory: 65535
```

which will set the memory limit to 64MB
