<div style="page-break-after: always;"></div>

## v1.23.0

Date: 2026-07-15

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.23.0

### Container Image

docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.23.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:0.21.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:0.21.0

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.23.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/v1.23.0/docs/src/upgrade

### Highlights

- Renamed the `snapshot-controller-4` chart to `snapshot-controller` and removed the deprecated `snapshot-controller-3` chart.
- Updated the snapshot-controller.
- Fixed a discovery-client issue that could cause the manual `connect-all` NVMe connection command to fail unexpectedly.
