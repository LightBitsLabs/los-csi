# LightOS CSI Plugin (`lb-csi-plugin`)

- [LightOS CSI Plugin (`lb-csi-plugin`)](#lightos-csi-plugin-lb-csi-plugin)
  - [Introduction](#introduction)
  - [Documentation](#documentation)
  - [Change Log](#change-log)
  - [About Lightbits Labs™](#about-lightbits-labs)

## Introduction

The LightOS CSI plugin is a software module that implements management of persistent storage volumes exported by LightOS software for Container Orchestrator (CO) systems like Kubernetes and Mesos. In conjunction with the LightOS disaggregated storage solution, the CSI plugin provides a building block for the easy deployment of stateful containerized applications on CO clusters.

The version of the plugin covered by this document implements version 1.2 of the [Container Storage Interface (CSI) Specification](https://github.com/container-storage-interface/spec/blob/v1.2.0/spec.md), and is compatible with LightOS version 2.2.x

## Documentation

Documentation can be found [here](./docs/deployment.md)

## Change Log

See the [CHANGELOG](./docs/CHANGELOG/README.md) for a detailed description of changes
between `lb-csi-plugin` versions.

## About Lightbits Labs™

Today's storage approaches were designed for enterprises and do not meet developing cloud-scale infrastructure requirements. For instance, SAN is known for lacking performance and control. At scale, Direct-Attached SSDs (DAS) have become too complicated for smooth operations, too costly, and suffer from inefficient SSD utilization.
Cloud-scale infrastructures require disaggregation of storage and compute, as evidenced by the top cloud giants’ transition from inefficient Direct-Attached SSD architecture to low-latency shared NVMe flash architecture. 

Unlike other NVMe-oF approaches, the Lightbits NVMe/TCP cost-saving solution separates storage and compute without touching network infrastructure or data center clients.
The Lightbits team members were key contributors to the NVMe standard and among the originators of NVMe over Fabrics (NVMe-oF). Now, Lightbits is crafting the new NVMe/TCP standard.
As the trailblazers in this field, the Lightbits solution is already successfully tested in industry-leading cloud data centers.
The company’s shared NVMe architecture provides efficient and robust disaggregation. With a transition that is so smooth, your applications teams won’t even notice the change. They can now go wild with better tail latency than local SSDs! 

And finally, you can separate storage from compute without drama.
