<div style="page-break-after: always;"></div>

## v1.24.0

Date: 2026-10-06

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.24.0

### Container Images

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.24.0
- docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:v1.24.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:0.22.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:0.22.0

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.24.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/v1.24.0/docs/src/upgrade

### Highlights

- IP-ACL enforcement support: the new opt-in `ip-acl` StorageClass
  parameter makes the plugin manage each volume's IP-ACL - the node
  plugin enrolls the node's data-path addresses at stage time and the
  IP-ACL is reset when the last node detaches. Statically provisioned
  volumes opt in with the `|ipacl:enabled` volumeHandle suffix, and the
  workload-examples charts expose the parameter end to end.
- NVMe in-band authentication (DH-HMAC-CHAP): with the new `inBandAuth`
  Helm value, each node self-registers as a trusted host on the
  Lightbits cluster, its secret pair is generated server-side and
  maintained in the discovery-client configuration, and every NVMe/TCP
  connection authenticates. Requires the in-container discovery-client;
  the chart refuses to render otherwise.
- The bundled discovery-client fixes a goroutine leak on discovery
  connection teardown that grew memory use on hosts with unreachable or
  flapping discovery endpoints.
- Static manifests: a new in-band auth flavor,
  `lb-csi-plugin-k8s-v<ver>-dc-iba.yaml`, ships for Kubernetes v1.33
  and v1.35.

