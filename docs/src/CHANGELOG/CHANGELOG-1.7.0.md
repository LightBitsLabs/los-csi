<div style="page-break-after: always;"></div>
\pagebreak

## v1.7.0

Date: 2021-11-05

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.7.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.7.0`

### Helm Charts

- `docker.lightbitslabs.com/lightos-csi/lb-csi:v0.5.0`
- `docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.5.0`

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.7.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/duros/docs/upgrade

### Highlights

- Implement NodeGetVolumeStats. (issue: LBM1-17861)
- Add xfs support. (issue: LBM1-12627)
- Add image for building lb-csi plugin and all related artifacts. Edit Makefile to run build/test/package targets in that image.
- Port lb-csi-plugin image to Alpine:3.14 base image.
