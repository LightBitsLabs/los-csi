<div style="page-break-after: always;"></div>
\pagebreak

# LightOS CSI Plugin Deployment

- [LightOS CSI Plugin Deployment](#lightos-csi-plugin-deployment)
    - [Before You Begin](#before-you-begin)
    - [Installing and Configuring the `kubectl` Tool](#installing-and-configuring-the-kubectl-tool)
    - [Ensuring Suitable Kubernetes Cluster Configuration](#ensuring-suitable-kubernetes-cluster-configuration)
      - [Configuring the Kubernetes API Server](#configuring-the-kubernetes-api-server)
      - [Configuring `kubelet` on All Cluster Nodes](#configuring-kubelet-on-all-cluster-nodes)
    - [Lightbits CSI Bundle Package](#lightbits-csi-bundle-package)
      - [Download Lightbits CSI Bundle Package](#download-lightbits-csi-bundle-package)
      - [Lightbits CSI Bundle Package Content](#lightbits-csi-bundle-package-content)
    - [Accessing the Docker Registry Hosting the Lightbits CSI Plugin](#accessing-the-docker-registry-hosting-the-lightbits-csi-plugin)
      - [Deploying from the Lightbits Image Registry](#deploying-from-the-lightbits-image-registry)
      - [Deploying from a Local Private Docker Registry](#deploying-from-a-local-private-docker-registry)

> **Note:**
> 
> To deploy the Lightbits CSI plugin, complete the steps in the order they appear.

> **Note:**
> 
> Due to the ongoing Kubernetes API and feature gates changes, the deployment instructions for different Kubernetes versions differ in various minor ways.
> 
> The instructions below cover deployment flows for the following environments:
> 
> * Deployment on Kubernetes versions between v1.17 and v1.22.x, inclusive.
> 
> When deploying the Lightbits CSI plugin, only one set of version-specific instructions for every section needs to be carried out, matching the target Kubernetes environment version.

### Before You Begin

To access and mount the storage volumes exported by the LightOS clusters, each of the Kubernetes cluster nodes MUST already have installed: 

- The appropriate Linux kernel.
- The NVMe/TCP transport kernel module.

The `Discovery-Client` service SHOULD be deployed if we want to run it on the host and not inside the `lb-csi-node` POD.

To ensure smooth access to the storage volumes exported by the LightOS clusters, the Kubernetes cluster nodes should be configured to automatically load the NVMe/TCP kernel module on Kubernetes node restart.

> Note:
> 
> To verify that the required software is installed and properly configured, or to obtain the required software, see the LightOS v2.3.16 Installation Guide document.


Before deploying the Lightbits CSI plugin, you should verify that:

- The LightOS storage cluster is in a "Healthy" state according to its management API service or CLI.
- It is possible to manually mount volumes exported over NVMe/TCP by each of the appropriate LightOS storage cluster servers on each of the Kubernetes cluster nodes.
- It is possible to manually access the LightOS management API service instances running on each of the appropriate LightOS storage cluster servers from each of the Kubernetes cluster nodes; e.g., using curl.
- The Discovery-Client service is started and enabled on each Kubernetes worker node, in case we choose to run it on the host.

Security systems like network firewalls and Linux Mandatory Access Control (MAC) systems (e.g., SELinux, AppArmor) running on the Kubernetes cluster nodes or the LightOS cluster servers can interfere with proper operation of the Lightbits CSI plugin. You should disable these security systems or configure them to allow the CSI plugin to carry out the requisite actions. For more details, see the Lightbits CSI Plugin Release Notes document appropriate to the version of the CSI plugin you are deploying.

### Installing and Configuring the `kubectl` Tool

Deployment of the Lightbits CSI plugin can be carried out from any computer—including user laptops, Kubernetes cluster nodes and other servers—with network access to the Kubernetes API server and to the Kubernetes cluster control plane using the standard [kubectl](https://kubernetes.io/docs/reference/kubectl/overview/) tool. If `kubectl` is not already installed on the machine in question, please install it before proceeding to the next subsection.

For full `kubectl` installation instructions and configuration, including ways of obtaining the relevant kubeconfig file for the Kubernetes cluster and instructing `kubectl` to use this kubeconfig file, please consult the [Install and Set Up `kubectl`](https://kubernetes.io/docs/tasks/tools/install-kubectl/) section of the Kubernetes documentation.

Verify that `kubectl` is configured and can communicate with the right Kubernetes cluster by executing the following commands (please note that the details of the output you will see will likely differ):

```bash
$ kubectl cluster-info 
Kubernetes master is running at https://1.2.3.4:6443
KubeDNS is running at https://1.2.3.4:6443/api/v1/namespaces/kube-system/services/kube-dns:dns/proxy
kubernetes-dashboard is running at https://1.2.3.4:6443/api/v1/namespaces/kube-system/services/https:kubernetes-dashboard:/proxy

To further debug and diagnose cluster problems, use 'kubectl cluster-info dump'.

$ kubectl get nodes
NAME      STATUS    ROLES     AGE       VERSION
node1     Ready     master    90d       v1.17.5
node2     Ready     node      90d       v1.17.5
node3     Ready     node      90d       v1.17.5
```

If the `kubectl` output includes errors, or if `kubectl` is unable to retrieve the Kubernetes cluster nodes information, resolve the issue using standard `kubectl` troubleshooting procedures before proceeding.

### Ensuring Suitable Kubernetes Cluster Configuration

CSI plugins must be able to perform system-level operations to provide other pods running on Kubernetes with usable storage volumes - either raw block devices or file system mounts. Some of these operations, like mounting file systems or accessing the sysfs pseudo-file system, require elevated privileges not normally granted to the other pods.

Therefore, to successfully deploy any CSI storage plugin—including the Lightbits CSI plugin—on a Kubernetes cluster, some security-related restrictions on Kubernetes pods must be relaxed by allowing Kubernetes to run privileged pods. Also,  depending on a Kubernetes version, a number of Kubernetes “feature gates” might need to be enabled or disabled. Details about configuring the restrictions and feature gates are in sections [Ensure Suitable Kubernetes API Server Configuration](#configuring-the-kubernetes-api-server) and [Ensure Suitable kubelet Configuration](#configuring-kubelet-on-all-cluster-nodes) on All Cluster Nodes.

> Note:
> 
> Different Kubernetes versions can have different security settings and different “feature gates” enabled or disabled by default.

Additionally, different methods of deploying Kubernetes clusters may enable or disable feature gates or security features in non-standard ways, and some Kubernetes users modify some of the relevant settings of their clusters after deployment. You must confirm that the settings described in this section are set to the correct values on the Kubernetes cluster in question.

The required modifications apply to two Kubernetes components:

- The Kubernetes API server.
- The kubelet daemon runs on each cluster node.

The following sections detail the required modifications for each component.

#### Configuring the Kubernetes API Server

In many Kubernetes deployments, the Kubernetes API server itself runs as a static pod on the master node(s). The process of enabling privileged containers and feature gates in the API server requires updating the API server pod Kubernetes spec and ensuring that the Kubernetes API server restarts. Typically, the spec location is at the following path on the master node(s):

```bash
/etc/kubernetes/manifests/kube-apiserver.yaml
```

If your Kubernetes cluster uses a different method of hosting the Kubernetes API server, or if the master node(s) in your Kubernetes cluster does not have the `kube-apiserver.yaml` file at the indicated path, employ one of the following suggestions:

- See the Kubernetes deployment tooling documentation used to deploy your cluster.
- Contact Lightbits support for assistance.

To enable privileged containers and feature gates in the Kubernetes API server, open the kube-apiserver.yaml in a text editor, and make sure that the command spec entry of the Kubernetes API server container (spec.containers[0].command) conforms to the following requirements:

  - The --allow-privileged command line parameter:
  
    - Starting with Kubernetes v1.13, privileged pods are enabled by default in Kubernetes.
    - If this parameter is present in the file, it should be specified only once, with a value of true.

      ```bash
      --allow-privileged=true
      ```

    - If this parameter is absent - do not add it, it will take on the value of true by default, as it should.

  - In all supported Kubernetes versions (v1.16 and later), the below five --feature-gates parameter entries are enabled by default. If this parameter is present and includes any of the following feature gates, they must be set to a value of true or removed from the list:

    ```bash
    BlockVolume=true
    CSIBlockVolume=true
    CSIPersistentVolume=true
    KubeletPluginsWatcher=true
    MountPropagation=true
    ```

    If the --feature-gates command line parameter is absent or does not include any of these feature gates, they will take on the value of true by default, as they should.

The below shows part of the `kube-apiserver.yaml` for reference:

```yaml
apiVersion: v1
kind: Pod
metadata:
  annotations: 
    scheduler.alpha.kubernetes.io/critical-pod: '' 
  creationTimestamp: null
  labels: 
    component: kube-apiserver
    tier: control-plane 
  name: kube-apiserver
  namespace: kube-system
spec:
  containers:
  - command:
    - kube-apiserver
    ...
    - --allow-privileged=true
    ...
    - --feature-gates=CSIDriverRegistry=true,CSINodeInfo=true
    image: k8s.gcr.io/kube-apiserver:v1.22.5
    ...
```

If you have made any changes to the `kube-apiserver.yaml` file, save and close the file. The kubelet daemon on that Kubernetes node should be monitoring the static pod manifests directory, detect the modification, and automatically restart the Kubernetes API server container with the updated parameters. 

> Note
> 
> The Kubernetes API server might become unresponsive to `kubectl` commands for several seconds while the API server pod restarts. This unresponsive status is by design, as long as `kubectl` becomes fully operational again within a minute.

#### Configuring `kubelet` on All Cluster Nodes

In most Kubernetes deployments, `kubelet` runs as a daemon or service on each of the cluster nodes. The process of enabling privileged containers and feature gates in kubelet comprises updating the kubelet configuration on each of the Kubernetes cluster nodes and ensuring that all the relevant kubelet daemons are restarted.

Unfortunately, the method of passing command-line parameters to the `kubelet` daemon depends to a significant degree on the OS running on the Kubernetes cluster nodes, the “init” system used to manage services on those nodes, and the method used to deploy the Kubernetes cluster. The following instructions are for one sample way to update the `kubelet` configuration. You will need to consult the documentation for the tool used to deploy your Kubernetes cluster for instructions pertinent to your Kubernetes cluster. If you require further assistance, contact Lightbits support.

To perform this configuration update, you need to connect to each of the Kubernetes cluster nodes; e.g., using SSH, and ensuring that the kubelet startup script or configuration file passes in the relevant command-line flags. As indicated in the preceding note, the exact location of such a startup script or configuration can vary. Often the kubelet parameters are controlled by a number of environment variables that can be set in one or more of the files that are picked up by the OS component used to launch the kubelet service.

For instance, many RHEL/CentOS v7.x based deployments using `systemd` use one or more of the following files to configure kubelet:

```bash
/etc/sysconfig/kubelet
/etc/default/kubelet
/etc/kubernetes/kubelet.env
```

Some systems can have a dedicated `KUBE_ALLOW_PRIV` environment variable reserved for just that use case in such files. Other systems use the more generic `KUBELET_ARGS` or `KUBELET_EXTRA_ARGS`, and some systems might have both or neither.

In any case, after you have identified the method used to pass the command-line parameters to kubelet, you need to confirm that the following parameters match the requirements specified in the Kubernetes API Server Configuration subsection:

- The --allow-privileged command line parameter.
- The --feature-gates command line parameter and the comma-separated list of its values.

If you had to make any changes to the kubelet configuration, you must restart the kubelet service on the affected Kubernetes cluster node—or reboot the node altogether—to complete the process. No changes take effect until you restart the kubelet service.

Often there are multiple locations or environment variables used to specify the kubelet command-line parameter. Therefore, to avoid misconfiguration, after restarting kubelet check that the command-line flags listed above are passed to kubelet only once and with correct values, or not passed at all if you are relying on defaults instead. For example, you can use the following command and examine its output to see the actual parameters passed, and verify their presence of the correct parameters:

```bash
ps aux | grep kubelet
```

> Note:
>
> If you add Kubernetes nodes after the initial CSI plugin deployment process, you must ensure the correct kubelet daemon configuration on every new Kubernetes node to enable proper Lightbits CSI plugin operation.

### Lightbits CSI Bundle Package

Lightbits supplies an optional supplementary package that contains the configuration files used for Lightbits CSI plugin deployment, as well as some Persistent Volume usage example files.

#### Download Lightbits CSI Bundle Package

The link to the supplementary package should be similar to:

```bash
curl -l -s -O https://dl.lightbitslabs.com/public/lightos-csi/raw/files/lb-csi-bundle-<version>.tar.gz
```

#### Lightbits CSI Bundle Package Content

The `lb-csi-bundle` includes the following content:

```bash
.
├── examples
│   ├── block-workload.yaml
│   ├── filesystem-workload.yaml
│   ├── preprovisioned-block-workload.yaml
│   ├── preprovisioned-filesystem-workload.yaml
│   ├── secret-and-storage-class.yaml
│   ├── snaps-example-pvc-workload.yaml
│   ├── snaps-example-snapshot-class.yaml
│   ├── snaps-pvc-from-pvc-workload.yaml
│   ├── snaps-pvc-from-snapshot-workload.yaml
│   ├── snaps-snapshot-from-pvc-workload.yaml
│   └── statefulset-workload.yaml
├── helm
│   └── charts
│       ├── lb-csi-plugin-0.6.0.tgz
│       ├── lb-csi-workload-examples-0.6.0.tgz
│       ├── snapshot-controller-3-0.6.0.tgz
│       └── snapshot-controller-4-0.6.0.tgz
├── k8s
│   ├── lb-csi-plugin-k8s-v1.17-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.17.yaml
│   ├── lb-csi-plugin-k8s-v1.18-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.18.yaml
│   ├── lb-csi-plugin-k8s-v1.19-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.19.yaml
│   ├── lb-csi-plugin-k8s-v1.20-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.20.yaml
│   ├── lb-csi-plugin-k8s-v1.21-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.21.yaml
│   ├── lb-csi-plugin-k8s-v1.22-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.22.yaml
│   ├── snapshot-controller-3.yaml
│   └── snapshot-controller-4.yaml
```

- **k8s:** Contains static manifests to deploy `lb-csi-plugin` on various Kubernetes versions.
- **examples:** Provides various workload examples that use `lb-csi` as persistent storage backend using static manifests.
- **helm/charts:** Contain two Helm Charts:
  - **lb-csi-plugin-<CHART_VERSION>.tgz:** Provides a customizable way to deploy `lb-csi-plugin` using Helm on various Kubernetes versions using Helm Chart.
  - **lb-csi-workload-examples-<CHART_VERSION>.tgz:** Provides various workload examples that use `lb-csi` as persistent storage backend using Helm Chart.
  - **snapshot-controller-3-<CHART_VERSION>.tgz:** Chart to deploy VolumeSnapshot CRDs and Snapshot-Controller deployment for k8s versions < v1.20. A package that automates the process documented [here](https://kubernetes-csi.github.io/docs/snapshot-controller.html)
  - **snapshot-controller-4-<CHART_VERSION>.tgz:** Chart to deploy VolumeSnapshot CRDs and Snapshot-Controller deployment for k8s versions >= v1.20. A package that automates the process documented [here](https://kubernetes-csi.github.io/docs/snapshot-controller.html)

> Note
> 
> The provided deployment spec files are examples only. While they include a rudimentary set of Kubernetes Service Account, Role, Binding, etc... definitions required to deploy a fully functional Lightbits CSI plugin, the plugin users are expected to significantly refine and extend them to match their production-grade deployment requirements.
>
> This is especially true of the various security-related Kubernetes features, including Pod Security Policies.

### Accessing the Docker Registry Hosting the Lightbits CSI Plugin

Depending on the version of Kubernetes and the deployment configuration, deploying the Lightbits CSI plugin requires a total of 5 to 8 Docker container images, which must be made available from a Docker registry during deployment time.

The Lightbits CSI plugin can be deployed directly from the Lightbits Docker registry and the official Kubernetes sidecar container images Docker registry, or locally staged on a private Docker registry hosted within your organizational network. The locally staged method is typically only required if the Kubernetes cluster you are working with does not have access to the Internet.

#### Deploying from the Lightbits Image Registry

The Lightbits CSI plugin itself is packaged as a single container image that is deployed in pods on the Kubernetes cluster nodes. This container image is available through the following Lightbits Image Registry:

```bash
docker.lightbitslabs.com/lightos-csi
```

If you cannot access the Lightbits Docker registry, contact Lightbits support to obtain the latest Lightbits Docker registry address and access credentials.

Furthermore, during the deployment 4-6 of the required Kubernetes sidecar container images will be obtained from the Quay registry:

```bash
k8s.gcr.io
```

#### Deploying from a Local Private Docker Registry

It is possible to use a local private Docker registry if you deploy the Lightbits CSI plugin on a Kubernetes cluster that does not have access to an external network. In this case, you must use standard Docker commands first to pull the Lightbits CSI plugin Docker image, as well as several of the standard Kubernetes [CSI “sidecar” container](https://kubernetes-csi.github.io/docs/sidecar-containers.html) images, to a machine with external network access. Then, using standard Docker commands, push them from that machine to the local Docker registry. 

> Note
>
> Consult the Lightbits CSI plugin deployment spec file (e.g., lb-csi-plugin-k8s-v1.17.yaml for deployment on Kubernetes v1.17) for the specific version numbers of the sidecar container images that are required for deployment.

Deployment using sidecar container images of versions other than those specified, or images with the tag latest, is not supported.

- The Lightbits CSI plugin container image is:

  ```bash
  docker.lightbitslabs.com/lightos-csi/lb-csi-plugin
  ```

- For deployment on Kubernetes versions between v1.17.0 and v1.22.x, since Snapshot support was added, the required Kubernetes sidecar containers are:

  ```bash
  k8s.gcr.io/sig-storage/csi-node-driver-registrar
  k8s.gcr.io/sig-storage/csi-provisioner
  k8s.gcr.io/sig-storage/csi-attacher
  k8s.gcr.io/sig-storage/csi-resizer
  k8s.gcr.io/sig-storage/csi-snapshotter
  ```

Once all the relevant container images are staged in the local Image registry, you must modify the Lightbits CSI plugin deployment spec file that you obtained as part of the Supplementary Package. Specifically:

- Update the image locations highlighted in the following spec snippets to point to your local Docker registry.
- Remove or modify the imagePullSecrets spec entry to match the local Docker registry credentials, if any are used.

```yaml
kind: StatefulSet
apiVersion: apps/v1
metadata:
  name: lb-csi-controller
              ...
      containers:
        - name: lb-csi-plugin
          image: docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.8.0
              ...
        - name: csi-provisioner
          image: k8s.gcr.io/sig-storage/csi-provisioner:v2.2.2
              ...
        - name: csi-attacher
          image: k8s.gcr.io/sig-storage/csi-attacher:v3.3.0
              ...
      imagePullSecrets:
      - name: lb-docker-reg-cred
              ...
kind: DaemonSet
apiVersion: apps/v1
metadata:
  name: lb-csi-node
              ... 
      containers:
        - name: lb-csi-plugin
          image: docker.lightbitslabs.com/lightos-csi/lb-csi-plugin:1.8.0
              ...
        - name: driver-registrar
          image: k8s.gcr.io/sig-storage/driver-registrar:v2.1.0
              ...
      imagePullSecrets:
      - name: lb-docker-reg-cred
```
