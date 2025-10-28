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

- Fixed a discovery-client issue where the kernel rejects the connections due to hostid mismatch by identifying existing NVMe controllers with the same hostnqn and and overriding the hostid to match these controllers. 

- Upgrading discovery-client from v3.14 or earlier to v3.15 or later while live NVMe connections exist may fail to connect additional NVMe controllers due to a kernel hostid mismatch. Existing controllers must be configured in the discovery-client via /etc/nvme/hostid or by specifying --hostid in the configuration file under /etc/discovery-client/discovery.d/.

- Fixed the issue with downloading Docker images from Lightbits’ public repository.

- Rename Docker images by appending 'UBI' to RHEL UBI-based container images.


