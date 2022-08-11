<div style="page-break-after: always;"></div>

# LightOS™ CSI Plugin (`lb-csi-plugin`)

- [LightOS™ CSI Plugin (`lb-csi-plugin`)](#lightos-csi-plugin-lb-csi-plugin)
  - [Introduction](#introduction)
  - [LB CSI Driver Capabilities](#lb-csi-driver-capabilities)

<div style="page-break-after: always;"></div>

## Introduction

The LightOS™ CSI plugin is a software module that implements the management of persistent storage volumes exported by LightOS™ software for Container Orchestrator (CO) systems such as Kubernetes and Mesos. In conjunction with the LightOS™ disaggregated storage solution, the CSI plugin provides a building block for the easy deployment of stateful containerized applications on CO clusters.

The version of the plugin covered by this document implements version 1.2 of the [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), and is compatible with LightOS™ version 3.0.1

The document summarizes the basic CSI plugin deployment and usage guidelines. For the compatibility notes, list of new features, changes, and known limitations of the Lightbits CSI plugin software, see the version-specific Release Notes supplied with the CSI plugin.

To successfully deploy the LightOS™ CSI plugin on Kubernetes, you must be familiar with the concepts and management systems of the LightOS™ software and Kubernetes. Once the system is configured, no knowledge of LightOS™ or the Lightbits CSI plugin is required to deploy workloads that consume storage provided by LightOS™ storage clusters to the Kubernetes cluster.
Unless you are already familiar with these topics, we recommend that you review the following Kubernetes documentation before you get started:

- [Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes)
- [Dynamic Volume Provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning)
- [StatefulSet Basics](https://kubernetes.io/docs/tutorials/stateful-application/basic-stateful-set)
- [Configure a Pod to Use a PersistentVolume for Storage](https://kubernetes.io/docs/tasks/configure-pod-container/configure-persistent-volume-storage)
- [Secrets](https://kubernetes.io/docs/concepts/configuration/secret)
- [Managing Secret using kubectl](https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl)

Kubernetes supports provisioning PVs either [dynamically]() or [statically](); i.e., using pre-provisioned volumes. The Lightbits CSI plugin supports both methods.

###	Lightbits™ Support

If you have any questions about the deployment, usage, or functionality of the Lightbits CSI plugin, contact Lightbits™ support by email `support@lightbitslabs.com` or visit our [customer support portal](https://lightbitslabs.atlassian.net/servicedesk/customer/portals).

<div style="page-break-after: always;"></div>
\pagebreak

## LB CSI Driver Capabilities

| Features                    | K8s v1.17 | K8s v1.18 | K8s v1.19 | K8s v1.20 | K8s v1.21 | K8s v1.22 |
|-----------------------------|-----------|-----------|-----------|-----------|-----------|-----------|
| Static Provisioning         | V         | V         | V         | V         | V         | V         |
| Dynamic Provisioning        | V         | V         | V         | V         | V         | V         |
| Expand Persistent Volume    | V         | V         | V         | V         | V         | V         |
| Create VolumeSnapshot       | V         | V         | V         | V         | V         | V         |
| Create Volume from Snapshot | V         | V         | V         | V         | V         | V         |
| Delete Snapshot             | V         | V         | V         | V         | V         | V         |
| CSI Volume Cloning          | V         | V         | V         | V         | V         | V         |
| CSI Raw Block Volume        | V         | V         | V         | V         | V         | V         |
| CSI Ephemeral Volume        | V         | V         | V         | V         | V         | V         |
| Topology                    | X         | X         | X         | X         | X         | X         |
| Access Mode                 | RWO       | RWO       | RWO       | RWO       | RWO       | RWO       |


- V: meaning feature is supported
- X: meaning feature is not supported
- RWO: [Read-Write-Once](https://kubernetes.io/docs/concepts/storage/persistent-volumes/#access-modes)