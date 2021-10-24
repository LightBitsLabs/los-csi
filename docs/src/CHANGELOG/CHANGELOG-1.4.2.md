<div style="page-break-after: always;"></div>
\pagebreak

## v1.4.2

Date: 2021-06-02

### Source code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.4.2

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.4.2`

### Documentation

https://github.com/LightBitsLabs/lb-csi/tree/v1.4.2/docs

### Upgrading

https://github.com/LightBitsLabs/lb-csi/tree/duros/docs/upgrade

### Highlights

- Stabilize `CreateSnapshot` API
- Validated with LightOS release v2.2.2

### All Changes

- On the CreateSnapshot API, lb-csi-controller managed to create the snapshot successfully but failed to return csi.CreateSnapshotResponse because of a nil reference. Since the panic was handled quietly, the lb-csi-controller kept running but the calling `external-snapshotter` never got the response. (LBM1-16702)
