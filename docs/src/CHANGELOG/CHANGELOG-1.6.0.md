<div style="page-break-after: always;"></div>
\pagebreak

## v1.6.0

Date: 2021-08-12

### Source Code

https://github.com/lightbitslabs/los-csi/releases/tag/v1.6.0

### Container Image

`docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:v1.6.0`

### Helm Charts

- `docker.lightbitslabs.com/lightos-csi/lb-csi:v0.4.0`
- `docker.lightbitslabs.com/lightos-csi/lb-csi-workload-examples:v0.4.0`

### Documentation

https://github.com/LightBitsLabs/los-csi/tree/v1.6.0/docs

### Upgrading

https://github.com/LightBitsLabs/los-csi/tree/duros/docs/upgrade

### Highlights

- Embed documentation inside source-control and expose documentation in PDF and HTML format.
- Drop `busybox` image dependency.
- Improve configuration of Discovery-Client when deployed in container.
- Expose Helm packages instead of bare Helm Charts.
- Unify prefix for all ClusterRoles and ClusterRoleBindings deployed.
