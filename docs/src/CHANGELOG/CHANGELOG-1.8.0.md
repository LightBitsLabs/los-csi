<div style="page-break-after: always;"></div>
\pagebreak

## v1.8.0

Date: 2021-11-05

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.8.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.8.0`

### Helm Charts

- `docker.lightbitslabs.com/lightos-csi/lb-csi:v0.6.0`
- `docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.6.0`

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.8.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/duros/docs/upgrade

### Highlights

- Drop v1.16 support.
- Add v1.22 support.
- Extract Snapshot-Controller and CRDs deployment to it's own Helm Chart. Remove the deployment of the Snapshot-Controller as a sidecar
  and deploy it as a stand alone deployment.
