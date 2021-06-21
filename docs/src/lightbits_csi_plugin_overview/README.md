<div style="page-break-after: always;"></div>

## Lightbits CSI Plugin Overview

For deployment on Kubernetes, CSI-conforming plugins are typically packaged as Docker container images that are deployed on the Kubernetes cluster using standard Kubernetes management primitives and tools.

The Lightbits CSI plugin is comprised of three logically separate services as required by the CSI specification:

- Controller Service
- Node Service
- Identity Service

For simplicity of deployment, all three services are packaged in the same Docker container image called lb-csi-plugin. The Identity Service is a supplementary one; it is active in each of the CSI plugin pod instances and will not be further mentioned in this document.

When deployed on Kubernetes clusters, Lightbits CSI plugin services run as an ensemble of regular Kubernetes pods. All instances of the CSI plugin services are themselves stateless. Kubernetes is responsible for scheduling the requisite pods, including ensuring the right number of healthy pods of the right kind are scheduled on the appropriate cluster nodes at any given time. The kinds of pods utilized by the CSI plugin are described in the [Controller Server](#controller-server) and [Node Server](#node-server) sections.

A single Lightbits CSI plugin deployment is required per Kubernetes cluster, regardless of the number of LightOS storage clusters that will be exporting storage volumes to the Kubernetes cluster nodes. All the information about the LightOS storage clusters and the volumes used for the various storage provisioning operations is typically transparently communicated between Kubernetes and the CSI plugin based on the standard Kubernetes storage primitives involved, such as PersistentVolumeClaims, StorageClasses, etc.

