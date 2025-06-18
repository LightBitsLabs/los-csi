<div style="page-break-after: always;"></div>

## v1.20.0

Date: 2025-07-16

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.20.0

### Container Image

docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.20.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:0.18.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:0.18.0

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.20.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/v1.20.0/docs/src/upgrade

### Highlights

- Support for discovery-client version 1.20.0 
  - Fixed an issue in discovery-client where new or pending requests were rejected after a connection timeout
  - Reconfigured discovery-client to remove CPU affinity from core 0 and use default settings.
- Support for kubernetes versions 1.31.x to 1.33.x



