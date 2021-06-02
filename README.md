# LightOS CSI Plugin (`lb-csi-plugin`)

- [LightOS CSI Plugin (`lb-csi-plugin`)](#lightos-csi-plugin-lb-csi-plugin)
  - [LB CSI Driver Capabilities](#lb-csi-driver-capabilities)
  - [Change Log](#change-log)
  - [Plugin and Workload Deployment On Kubernetes](#plugin-and-workload-deployment-on-kubernetes)
  - [Plugin Upgrade](#plugin-upgrade)
  - [Design and architecture](#design-and-architecture)
  - [Developing The Plugin](#developing-the-plugin)


The CSI Drivers by Lightbits implement an interface between CSI (CSI spec v1.2) enabled Container Orchestrator (CO) and LightOS Storage Cluster. It is a plug-in that is installed into Kubernetes to provide persistent storage using LightOS Storage Cluster.

## LB CSI Driver Capabilities

| Features	                      | K8s v1.15	| K8s v1.16	| K8s v1.17 | K8s v1.18 | K8s v1.19 | K8s v1.20 | K8s v1.21 |
|---------------------------------|-----------|-----------|-----------|-----------|-----------|-----------|-----------|
| Static Provisioning	            | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Dynamic Provisioning            | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Expand Persistent Volume	      | no	      | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Create VolumeSnapshot	          | no	      | no	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Create Volume from Snapshot	    | no	      | no	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Delete Snapshot	                | no	      | no	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| CSI Volume Cloning	            | no	      | no	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| CSI Raw Block Volume	          | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| CSI Ephemeral Volume	          | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      | yes	      |
| Topology	                      | no	      | no	      | no	      | no	      | no	      | no	      | no	      |
| Access Mode	                    | RWO       | RWO       | RWO       | RWO       | RWO       | RWO       | RWO       |

## Change Log

See the [CHANGELOG](./docs/CHANGELOG/README.md) for a detailed description of changes
between `lb-csi-plugin` versions.

## Plugin and Workload Deployment On Kubernetes

See [docs/deployment.md](./docs/deployment.md)

## Plugin Upgrade

See [docs/upgrade/upgrade-lb-csi.md](./docs/upgrade/upgrade-lb-csi.md)

## Design and architecture

See [docs/design.md](./docs/design.md)

## Developing The Plugin

See [docs/develop.md](./docs/develop.md)
