<div style="page-break-after: always;"></div>

## v1.24.1

Date: 2026-11-05

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.24.1

### Container Images

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.24.1
- docker.lightbitslabs.com/lightos-csi/lb-nvme-discovery-client:v1.24.1

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:0.22.1
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:0.22.1

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.24.1/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/v1.24.1/docs/src/upgrade

### Highlights

- The controller deploys the `csi-attacher` sidecar at v4.9.0 instead
  of v4.8.0. The plugin itself does not change.
