# LightOS CSI Plugin (`lb-csi-plugin`)

- [LightOS CSI Plugin (`lb-csi-plugin`)](#lightos-csi-plugin-lb-csi-plugin)
  - [Abbreviations and Terms](#abbreviations-and-terms)
  - [Introduction](#introduction)
  - [LB CSI Driver Capabilities](#lb-csi-driver-capabilities)
  - [Change Log](#change-log)
      - [Discovery-Client](#discovery-client)
  - [Plugin and Workload Deployment On Kubernetes](#plugin-and-workload-deployment-on-kubernetes)
  - [Plugin Upgrade](#plugin-upgrade)
  - [Design and architecture](#design-and-architecture)
  - [Developing The Plugin](#developing-the-plugin)
  - [External References](#external-references)
  - [About Lightbits Labs™](#about-lightbits-labs)

## Abbreviations and Terms

| Abbreviations | Description                                                                                           |
| ------------- | ----------------------------------------------------------------------------------------------------- |
| CSI           | Container Storage Interface, a specification for containerized applications storage volume management |
| CO            | Container Orchestrator, e.g. Kubernetes                                                               |
| DS            | Kubernetes Daemon Set (DaemonSet)                                                                     |
| LightOS®      | Lightbits™ software-defined disaggregated storage solution                                            |
| NIC           | Network Interface Card                                                                                |
| NUMA          | Non-Uniform Memory Access                                                                             |
| PV            | Kubernetes Persistent Volume (PersistentVolume)                                                       |
| PVC           | Kubernetes Persistent Volume Claim (PersistentVolumeClaim)                                            |
| RPM           | RPM Package Manager                                                                                   |
| SC            | Kubernetes Storage Class (StorageClass)                                                               |
| STS           | Kubernetes Stateful Set (StatefulSet)                                                                 |

## Introduction

The LightOS CSI plugin is a software module that implements management of persistent storage volumes exported by LightOS software for Container Orchestrator (CO) systems like Kubernetes and Mesos. In conjunction with the LightOS disaggregated storage solution, the CSI plugin provides a building block for the easy deployment of stateful containerized applications on CO clusters.

The version of the plugin covered by this document implements version 1.2 of the [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), and is compatible with LightOS version 2.2.x

The document summarizes the basic CSI plugin deployment and usage guidelines. For the compatibility notes, list of new features, changes, and known limitations of the Lightbits CSI plugin software, see the version-specific Release Notes supplied with the CSI plugin.

To successfully deploy the LightOS CSI plugin on Kubernetes, you must be familiar with the concepts and management systems of the LightOS software and Kubernetes. Once the system is configured, no knowledge of LightOS or the Lightbits CSI plugin is required to deploy workloads that consume storage provided by LightOS storage clusters to the Kubernetes cluster.
Unless you are already familiar with these topics, we recommend that you review the following Kubernetes documentation before you get started:

- [Persistent Volumes](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Storage Classes](https://kubernetes.io/docs/concepts/storage/storage-classes)
- [Dynamic Volume Provisioning](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning)
- [StatefulSet Basics](https://kubernetes.io/docs/tutorials/stateful-application/basic-stateful-set)
- [Configure a Pod to Use a PersistentVolume for Storage](https://kubernetes.io/docs/tasks/configure-pod-container/configure-persistent-volume-storage)
- [Secrets](https://kubernetes.io/docs/concepts/configuration/secret)
- [Managing Secret using kubectl](https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl)

Kubernetes supports provisioning PVs either [dynamically]() or [statically](); i.e. using pre-provisioned volumes. The Lightbits CSI plugin supports both methods.

###	Lightbits™ Support

If you have any questions about the deployment, usage, or functionality of the Lightbits CSI plugin, contact Lightbits™ support by email `support@lightbitslabs.com` or visit our [customer support portal](https://lightbitslabs.atlassian.net/servicedesk/customer/portals).

## LB CSI Driver Capabilities

| Features                    | K8s v1.15          | K8s v1.16          | K8s v1.17          | K8s v1.18          | K8s v1.19          | K8s v1.20          | K8s v1.21          |
| --------------------------- | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ | ------------------ |
| Static Provisioning         | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Dynamic Provisioning        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Expand Persistent Volume    | :x:                | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Create VolumeSnapshot       | :x:                | :x:                | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Create Volume from Snapshot | :x:                | :x:                | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Delete Snapshot             | :x:                | :x:                | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| CSI Volume Cloning          | :x:                | :x:                | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| CSI Raw Block Volume        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| CSI Ephemeral Volume        | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: | :heavy_check_mark: |
| Topology                    | :x:                | :x:                | :x:                | :x:                | :x:                | :x:                | :x:                |
| Access Mode                 | RWO                | RWO                | RWO                | RWO                | RWO                | RWO                | RWO                |

## Change Log

See the [CHANGELOG](./docs/CHANGELOG/README.md) for a detailed description of changes
between `lb-csi-plugin` versions.

##	Prerequisites

Before deploying with the Lightbits CSI plugin, you must have the following items working in your environment and must be familiar with the aspects of Kubernetes operation.

| Requirement                                         | Description                                                                                                                                                                                                                                                                                                                                                                                                      |
| --------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| LightOS 2.0 storage cluster                         | One or more fully configured and functional LightOS storage clusters with sufficient free storage capacity.                                                                                                                                                                                                                                                                                                      |
| Kubernetes cluster                                  | A fully configured and functional Kubernetes cluster.                                                                                                                                                                                                                                                                                                                                                            |
| Administrative privileges on the Kubernetes cluster | Some of the steps required to deploy the Lightbits CSI plugin require administrative privileges—including root or equivalent access—on each of the hosts that act as Kubernetes cluster members, as well as administrative privileges for Kubernetes management. By default, the subsequent creation or usage of Persistent Volumes managed by the CSI plugin requires no elevated privileges.                   |
| Kubernetes management expertise                     | Deploying storage plugins—including plugins conforming to the CSI specification—on Kubernetes requires a thorough understanding of Kubernetes, its concepts, services, best practices, configuration, security considerations and troubleshooting flows.                                                                                                                                                         |
| Docker registry access                              | By default, the Lightbits CSI plugin is deployable  from an externally accessible Lightbits Docker registry. Alternatively, if the Kubernetes cluster on which the CSI plugin is to be deployed does not have external network access, the CSI plugin can be downloaded using a host with external network access. The plugin can then be deployed to your internal private Docker registry.                     |
| Network connectivity                                | Each of the Kubernetes cluster nodes must have full network connectivity to all the NICs of each of the LightOS cluster servers that are configured to serve either the LightOS management API service or the NVMe/TCP data endpoints. There is no requirement that all the services of the LightOS cluster servers be served on the same network, as long as the other connectivity requirements are satisfied. |

##	Lightbits CSI Plugin Overview

For deployment on Kubernetes, CSI-conforming plugins are typically packaged as Docker container images that are deployed on the Kubernetes cluster using standard Kubernetes management primitives and tools.

The Lightbits CSI plugin is comprised of three logically separate services as required by the CSI specification:

- Controller Service
- Node Service
- Identity Service

For simplicity of deployment, all three services are packaged in the same Docker container image called lb-csi-plugin. The Identity Service is a supplementary one; it is active in each of the CSI plugin pod instances and will not be further mentioned in this document.

When deployed on Kubernetes clusters, Lightbits CSI plugin services run as an ensemble of regular Kubernetes pods. All instances of the CSI plugin services are themselves stateless. Kubernetes is responsible for scheduling the requisite pods, including ensuring the right number of healthy pods of the right kind are scheduled on the appropriate cluster nodes at any given time. The kinds of pods utilized by the CSI plugin are described in the [Controller Server](#controller-server) and [Node Server](#node-server) sections.

A single Lightbits CSI plugin deployment is required per Kubernetes cluster, regardless of the number of LightOS storage clusters that will be exporting storage volumes to the Kubernetes cluster nodes. All the information about the LightOS storage clusters and the volumes used for the various storage provisioning operations is typically transparently communicated between Kubernetes and the CSI plugin based on the standard Kubernetes storage primitives involved, such as PersistentVolumeClaims, StorageClasses, etc.

###	Controller Server

The Lightbits CSI plugin’s Controller Server consists of a pod that includes the lb-csi-plugin container and several standard Kubernetes sidecar containers. A single Controller Server pod instance is deployed per Kubernetes cluster in a StatefulSet.

This pod communicates with the Kubernetes [API Server](https://kubernetes.io/docs/concepts/overview/components) and the LightOS management API service on the LightOS storage servers. It is responsible for PV lifecycle management, including:

- Creation and deletion of volumes on the LightOS cluster.
- Making these volumes accessible to the Kubernetes cluster nodes that consume the storage on an as-needed basis.

###	Node Server

The Lightbits CSI plugin's Node Server is a pod that includes the lb-csi-plugin and a standard Kubernetes sidecar container.  A single Node Server instance is deployed per Kubernetes cluster node using a DaemonSet. As such, the Node Server pod can optionally include a busybox-based [Init Container](https://kubernetes.io/docs/concepts/workloads/pods/init-containers) to auto-load the NVMe/TCP driver after the Kubernetes node reboots, though this functionality can be eschewed if an OS-level driver auto-loading mechanism is used instead.

Each Node Server pod communicates with the local kubelet daemon on its respective Kubernetes cluster node, as well as the LightOS management API service instances on the LightOS cluster servers. Their responsibilities include: 

- Making the storage volumes exported by the LightOS clusters accessible to the Kubernetes nodes.
- Formatting and checking the file system integrity of the volumes—if necessary.
- Making the volumes accessible to the specific workload pods scheduled to the cluster node in question.

When resizing the Kubernetes cluster by adding or removing cluster nodes, Kubernetes automatically manages the scheduling or termination of the CSI plugin Node Server pods on the affected cluster nodes. No operator intervention is required.

#### Discovery-Client

Each Node Server should run a Discovery-Client service as a daemon provided by Lightbits™, which enables dynamically connecting to new LightOS cluster nodes.

The lb-csi-plugin running on each node as part of the DaemonSet will communicate with the Discovery-Client and configure it to query one of the Discovery-Services running on the LightOS cluster.

The Discovery-Client role is critical since this process will issue a NVMe Connect command when needed and expose the volume on the ServerNode when needed.
Please refer to the LightOS Administration Guide for installation instructions for deploying the Discovery-Client.

## Plugin and Workload Deployment On Kubernetes

See [docs/deployment.md](./docs/deployment.md)

## Plugin Upgrade

See [docs/upgrade/upgrade-lb-csi.md](./docs/upgrade/upgrade-lb-csi.md)

## Design and architecture

See [docs/design.md](./docs/design.md)

## Developing The Plugin

See [docs/develop.md](./docs/develop.md)

## External References

- [Kubernetes project home](https://kubernetes.io)
- [Container Storage Interface (CSI) Specification, v1.2.0](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md)
- [Kubernetes Overview of kubectl documentation page](https://kubernetes.io/docs/reference/kubectl/overview/)
- [Kubernetes Install and Set Up kubectl documentation page](https://kubernetes.io/docs/tasks/tools/install-kubectl/)
- [Kubernetes Components documentation page](https://kubernetes.io/docs/concepts/overview/components)
- [Kubernetes Feature Gates documentation page](https://kubernetes.io/docs/reference/command-line-tools-reference/feature-gates)
- [Kubernetes Static Pods documentation page](https://kubernetes.io/docs/tasks/administer-cluster/static-pod)
- [Kubernetes Persistent Volumes documentation page](https://kubernetes.io/docs/concepts/storage/persistent-volumes/)
- [Kubernetes Configure a Pod to Use a PersistentVolume for Storage task documentation page](https://kubernetes.io/docs/tasks/configure-pod-container/configure-persistent-volume-storage)
- [Kubernetes Storage Classes documentation page](https://kubernetes.io/docs/concepts/storage/storage-classes)
- [Kubernetes StatefulSet Basics tutorial page](https://kubernetes.io/docs/tutorials/stateful-application/basic-stateful-set)
- [Kubernetes Dynamic Volume Provisioning documentation page](https://kubernetes.io/docs/concepts/storage/dynamic-provisioning)
- [Kubernetes Pull an Image from a Private Registry documentation page](https://kubernetes.io/docs/tasks/configure-pod-container/pull-image-private-registry/)
- [Kubernetes CSI Sidecar Containers documentation page](https://kubernetes-csi.github.io/docs/sidecar-containers.html)
- [PVC created by statefulset will not be auto removed Kubernetes GitHub issue #55045](https://github.com/kubernetes/kubernetes/issues/55045)
- [Kubernetes Troubleshoot Clusters documentation page](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-cluster)
- [Kubernetes Determine the Reason for Pod Failure documentation page](https://kubernetes.io/docs/tasks/debug-application-cluster/determine-reason-pod-failure)
- [Kubernetes Debug Pods and ReplicationControllers documentation page](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-pod-replication-controller)
- [Kubernetes Troubleshoot Applications documentation page](https://kubernetes.io/docs/tasks/debug-application-cluster/debug-application)
- [Kubernetes Safely Drain a Node while Respecting the PodDisruptionBudget documentation page](https://kubernetes.io/docs/tasks/administer-cluster/safely-drain-node)
- [Kubernetes Init Containers documentation page](https://kubernetes.io/docs/concepts/workloads/pods/init-containers)
- [IETF RFC 7519: JSON Web Token (JWT)](https://tools.ietf.org/html/rfc7519)
- [Kubernetes Secrets documentation page](https://kubernetes.io/docs/concepts/configuration/secret)
- [Kubernetes Managing Secret using kubectl documentation page](https://kubernetes.io/docs/tasks/configmap-secret/managing-secret-using-kubectl)

## About Lightbits Labs™

Today's storage approaches were designed for enterprises and do not meet developing cloud-scale infrastructure requirements. For instance, SAN is known for lacking performance and control. At scale, Direct-Attached SSDs (DAS) have become too complicated for smooth operations, too costly, and suffer from inefficient SSD utilization.
Cloud-scale infrastructures require disaggregation of storage and compute, as evidenced by the top cloud giants’ transition from inefficient Direct-Attached SSD architecture to low-latency shared NVMe flash architecture. 

Unlike other NVMe-oF approaches, the Lightbits NVMe/TCP cost-saving solution separates storage and compute without touching network infrastructure or data center clients.
The Lightbits team members were key contributors to the NVMe standard and among the originators of NVMe over Fabrics (NVMe-oF). Now, Lightbits is crafting the new NVMe/TCP standard.
As the trailblazers in this field, the Lightbits solution is already successfully tested in industry-leading cloud data centers.
The company’s shared NVMe architecture provides efficient and robust disaggregation. With a transition that is so smooth, your applications teams won’t even notice the change. They can now go wild with better tail latency than local SSDs! 

And finally, you can separate storage from compute without drama.
