<div style="page-break-after: always;"></div>
\pagebreak

### Controller Server

The Lightbits CSI plugin’s Controller Server consists of a pod that includes the lb-csi-plugin container and several standard Kubernetes sidecar containers. A single Controller Server pod instance is deployed per Kubernetes cluster in a StatefulSet.

This pod communicates with the Kubernetes [API Server](https://kubernetes.io/docs/concepts/overview/components) and the LightOS management API service on the LightOS storage servers. It is responsible for PV lifecycle management, including:

- Creation and deletion of volumes on the LightOS cluster.
- Making these volumes accessible to the Kubernetes cluster nodes that consume the storage on an as-needed basis.

