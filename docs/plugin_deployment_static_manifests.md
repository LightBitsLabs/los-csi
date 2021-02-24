# LightOS CSI Plugin Deployment Using Static Manifests

- [LightOS CSI Plugin Deployment Using Static Manifests](#lightos-csi-plugin-deployment-using-static-manifests)
  - [Overview](#overview)
  - [Deploying LightOS CSI Plugin On Kubernetes Cluster](#deploying-lightos-csi-plugin-on-kubernetes-cluster)
    - [Deploying LightOS CSI Plugin](#deploying-lightos-csi-plugin)
    - [CSI Plugin Removal Instructions](#csi-plugin-removal-instructions)
      - [Before Removing the CSI Plugin](#before-removing-the-csi-plugin)
    - [Removing LightOS CSI Plugin](#removing-lightos-csi-plugin)
  - [Deploying LB-CSI Plugin Via Helm (Optional)](#deploying-lb-csi-plugin-via-helm-optional)
  - [Next Steps](#next-steps)

## Overview

LightOS CSI plugin is packaged as a standard Kubernetes workload, using a StatefulSet and a DaemonSet. Therefore, the deployment process is as simple as a regular Kubernetes workload deployment, using regular Kubernetes manifests.

The following instructions demonstrate a simplified plugin deployment flow using the sample configuration and deployment specs from the Supplementary Package. For production usage, you can refer to the provided examples and develop your deployment flows to address your requirements.

**Note:**
  
> The scripts and spec files provided for your convenience in the Supplementary Package deploy the Lightbits CSI Plugin into the kube-system Kubernetes namespace, rather than the default one. Make sure to reference this namespace when issuing Kubernetes commands to confirm the successful installation.

> There is no technical requirement to keep the CSI plugin in the kube-system namespace for actual deployments. Since kube-system is a privileged Kubernetes namespace, this can often avoid unexpected loss of service due to operator mistakes.

## Deploying LightOS CSI Plugin On Kubernetes Cluster

Provided manifests create the required `ServiceAccount` and RBAC `ClusterRole`/`ClusterRoleBinding` Kubernetes objects.

Some of the features are not supported for some of the k8s versions. For example `Extend Volume` feature is supported for k8s v1.16 and above.

We provide a manifest file for each k8s version supported:

```bash
k8s/
├── lb-csi-plugin-k8s-v1.13.yaml
├── lb-csi-plugin-k8s-v1.15-dc.yaml
├── lb-csi-plugin-k8s-v1.15.yaml
├── lb-csi-plugin-k8s-v1.16-dc.yaml
├── lb-csi-plugin-k8s-v1.16.yaml
├── lb-csi-plugin-k8s-v1.17-dc.yaml
├── lb-csi-plugin-k8s-v1.17.yaml
├── lb-csi-plugin-k8s-v1.18-dc.yaml
└── lb-csi-plugin-k8s-v1.18.yaml
```

**NOTE:**

> Manifests with suffix `-dc.yaml` deploy discovery-client on k8s as a container in `lb-csi-node` DaemonSet.

### Deploying LightOS CSI Plugin

To deploy the plugin, run the following commands with examples as the current directory and with kubectl in your $PATH.

```bash
kubectl create -f lb-csi-plugin-k8s-v1.15.yaml
```

Ideally, the output should contain no error messages. If you see any, try to determine if the problem is with the connectivity to the Kubernetes cluster, the kubelet configuration, or some other minor issue.

After the above command completes, the deployment process can take between several seconds and several minutes, depending on the size of the Kubernetes cluster, load on the cluster nodes, network connectivity, etc.

After a short while, you can issue the following commands to verify the results. Your output will likely differ from the following example, including to reflect your Kubernetes cluster configuration, randomly generated pod names, etc.

```bash
$ kubectl get --namespace=kube-system statefulset lb-csi-controller
NAME                DESIRED   CURRENT   AGE
lb-csi-controller   1         1         4m

$ kubectl get --namespace=kube-system daemonsets lb-csi-node
NAME          DESIRED   CURRENT   READY     UP-TO-DATE   AVAILABLE   NODE SELECTOR   AGE
lb-csi-node   3         3         3         3            3           <none>          4m

$  kubectl get --namespace=kube-system pod --selector app=lb-csi-plugin -o wide
NAME                  READY     STATUS    RESTARTS   AGE       IP              NODE      NOMINATED NODE
lb-csi-controller-0   3/3       Running   0          1m        10.233.65.12    node3     <none>
lb-csi-node-6ptlf     2/2       Running   0          1m        192.168.20.20   node3     <none>
lb-csi-node-blc46     2/2       Running   0          1m        192.168.20.22   node4     <none>
lb-csi-node-djv7t     2/2       Running   0          1m        192.168.20.18   node2     <none>
```

### CSI Plugin Removal Instructions

#### Before Removing the CSI Plugin

Before removing the CSI plugin, you must confirm that the Lightbits CSI plugin is not in use by the Kubernetes cluster or any of the Kubernetes objects still live on that cluster. The kinds of objects of interest include:

- StatefulSet, PersistentVolume, PersistentVolumeClaim, StorageClass objects.
- Pods that use volumes obtained from Lightbits CSI plugin.
- Other pods that might be directly or indirectly dependent on the CSI plugin.

Failure to confirm that the Lightbits CSI plugin is not in use can result in some Kubernetes objects remaining stuck in “Terminating” or similar states, and require intrusive manual administrative intervention.

### Removing LightOS CSI Plugin

Assuming you have deployed Lightbits CSI plugin by following the instructions in the section [Deploying LightOS CSI Plugin](#deploying-lightos-csi-plugin), you can remove the CSI plugin from your Kubernetes cluster and confirm the removal by executing the following commands with examples as the current directory.

```bash
$ kubectl delete -f lb-csi-plugin-k8s-v1.15.yaml

$ kubectl get --namespace=kube-system statefulset lb-csi-controller
No resources found.
Error from server (NotFound): statefulsets.apps "lb-csi-controller" not found

$ kubectl get --namespace=kube-system daemonsets lb-csi-node
No resources found.
Error from server (NotFound): daemonsets.extensions "lb-csi-node" not found

$ kubectl get --namespace=kube-system pod --selector app=lb-csi-plugin
No resources found.
```

The “No resources found” errors for the last three commands are expected and confirm the successful removal of the CSI plugin from the Kubernetes cluster.

After Lightbits CSI plugin is removed from the Kubernetes cluster, some volumes created by Kubernetes using the CSI plugin may remain on the LightOS storage cluster and may need to be manually deleted using the LightOS management API or CLI.

## Deploying LB-CSI Plugin Via Helm (Optional)

Helm may be used to install the `lb-csi-plugin`.

Follow instructions [here](./plugin_deployment_using_helm.md)

## Next Steps

- [Workload Examples Deployment Using Static Manifests](./workload_examples_deployment_using_static_manifests.md).
- [Workload Examples Deployment Using Helm](./workload_examples_deployment_using_helm.md).
