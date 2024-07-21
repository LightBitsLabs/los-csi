<div style="page-break-after: always;"></div>
\pagebreak

## v1.16.0

Date: 2024-08-14

### Source Code

https://github.com/lightbitslabs/lb-csi/releases/tag/v1.16.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.16.0

### Helm Charts

- docker.lightbitslabs.com/lightos-csi/lb-csi:v0.14.0
- docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.14.0`

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.16.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/duros/docs/upgrade

### Highlights

- Fixed an issue of I/O errors and continuous pod terminating state, this fix will try to umount the 
  volume in unpublish and unstage state even if verifying unmount fails, enabling clean detaching of volume with reported I/O error.