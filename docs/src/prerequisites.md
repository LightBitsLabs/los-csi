<div style="page-break-after: always;"></div>
\pagebreak

# Prerequisites

Before deploying with the Lightbits CSI plugin, you must have the following items working in your environment and must be familiar with aspects of Kubernetes operation.

| Requirement                                      | Description                                                                         |
| ------------------------------------------------ | ----------------------------------------------------------------------------------- |
| LightOS 2.0 storage cluster                                     | A fully configured and functional LightOS storage cluster with sufficient free storage capacity.  |
| Kubernetes cluster                                              | A fully configured and functional Kubernetes cluster. |
| Administrative privileges on the Kubernetes cluster             | Some of the steps required to deploy the Lightbits CSI plugin require administrative privileges—including root or equivalent access—on each of the hosts that act as Kubernetes cluster members, as well as administrative privileges for Kubernetes management. By default, the subsequent creation or usage of Persistent Volumes managed by the CSI plugin requires no elevated privileges.   |
| Kubernetes management expertise                                 | Deploying storage plugins—including plugins conforming to the CSI specification—on Kubernetes requires a thorough understanding of Kubernetes, its concepts, services, best practices, configuration, security considerations and troubleshooting flows. |
| Docker registry access                                          | By default, the Lightbits CSI plugin is deployable  from an externally accessible Lightbits Docker registry. Alternatively, if the Kubernetes cluster on which the CSI plugin is to be deployed does not have external network access, the CSI plugin can be downloaded using a host with external network access. The plugin can then be deployed to your internal private Docker registry. |
| Network connectivity                                            | Each of the Kubernetes cluster nodes must have full network connectivity to all the NICs of each of the LightOS cluster servers that are configured to serve either the LightOS management API service or the NVMe/TCP data endpoints. There is no requirement that all the services of the LightOS cluster servers be served on the same network, as long as the other connectivity requirements are satisfied. |
