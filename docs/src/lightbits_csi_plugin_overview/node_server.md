<div style="page-break-after: always;"></div>

### Node Server

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
