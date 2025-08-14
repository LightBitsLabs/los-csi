<div style="page-break-after: always;"></div>

## v1.21.0

Date: 2025-09-10

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.21.0

### Container Image

docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.21.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:0.19.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:0.19.0

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.21.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/v1.21.0/docs/src/upgrade

### Highlights

- Fixed a discovery-client issue where the AuxSuffix was not appended during NVMe reconnection to the target for an auxiliary system

- Fixed the issue with downloading Docker images from Lightbits’ public repository.

- Rename Docker images by appending 'UBI' to RHEL UBI-based container images.


