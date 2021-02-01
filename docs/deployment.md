# `lb-csi-plugin` Deployment

- [`lb-csi-plugin` Deployment](#lb-csi-plugin-deployment)
  - [LightOS CSI Plugin Deployment](#lightos-csi-plugin-deployment)
    - [Deploying LightOS CSI Plugin](#deploying-lightos-csi-plugin)
    - [Removing LightOS CSI Plugin](#removing-lightos-csi-plugin)
  - [Loading the operator via Helm (Optional)](#loading-the-operator-via-helm-optional)

## LightOS CSI Plugin Deployment

Create the required ServiceAccount and RBAC ClusterRole/ClusterRoleBinding Kubernetes objects

Each kubernetes version has features promoted from alpha to beta to GA.

Some of the features are not supported for some of the k8s versions. For example `Extend Volume` feature is supported for k8s v1.16 and above.

We provide a manifest file for each k8s version supported.

```bash
├── k8s
│   ├── lb-csi-plugin-k8s-v1.13.yaml
│   ├── lb-csi-plugin-k8s-v1.15.yaml
│   ├── lb-csi-plugin-k8s-v1.16.yaml
│   ├── lb-csi-plugin-k8s-v1.17-dc.yaml
│   ├── lb-csi-plugin-k8s-v1.17.yaml
│   └── lb-csi-plugin-k8s-v1.18.yaml
```

### Deploying LightOS CSI Plugin

To deploy the plugin, run the following commands with examples as the current directory and with kubectl in your $PATH.

```bash
kubectl create -f lb-csi-plugin-k8s-v1.15.yaml
```

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

## Loading the operator via Helm (Optional)

Helm may be used to install the `lb-csi-plugin`.

Follow instructions [here](../deploy/helm/lb-csi/README.md)
