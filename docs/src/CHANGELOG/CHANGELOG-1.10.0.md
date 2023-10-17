<div style="page-break-after: always;"></div>
\pagebreak

## v1.10.0

Date: 2023-02-15

### Source Code

https://github.com/lightbitslabs/lb-csi/releases/tag/v1.10.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.10.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi:v0.8.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.8.0`

### Documentation

https://github.com/LightBitsLabs/lb-csi/tree/v1.10.0/docs

### Upgrading

https://github.com/LightBitsLabs/lb-csi/tree/duros/docs/upgrade

### Highlights

- discovery-client not specifying timeout on nvme-init connection, and hanging if service does not respond. [#5](https://github.com/LightBitsLabs/discovery-client/issues/5)
- discovery-client failed to update referrals and cache entries when it received a list of bad entries. [#4](https://github.com/LightBitsLabs/discovery-client/issues/4)
- support client-side encryption.
