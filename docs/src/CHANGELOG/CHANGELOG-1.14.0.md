<div style="page-break-after: always;"></div>
\pagebreak

## v1.14.0

Date: 2024-01-18

### Source Code

https://github.com/lightbitslabs/lb-csi/releases/tag/v1.14.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.14.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi:v0.12.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.12.0`

### Documentation

https://github.com/LightBitsLabs/lb-csi/tree/v1.14.0/docs

### Upgrading

https://github.com/LightBitsLabs/lb-csi/tree/duros/docs/upgrade

### Highlights

- Discovery client: set hostid during nvme connection establishment
- Add kmod to provide zstd support for newer Ubuntu kernels modules
- Fixed: CSI plugin killed due to OOM after restarting kubelet (https://github.com/LightBitsLabs/los-csi/issues/33)

