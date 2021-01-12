# Snapshots and Clones example

Shows the snapshots and clones functionality.
First, we create a PVC and a pod which attaches to it and keeps writing the date to `/mnt/test/log`.
Then, we clone the PVC either by first creating a snapshot and cloning from the snapshot, or by cloning from the PVC directly, and start another pod which slepps forever.
We can see that the cloning was succeessful by attaching to each of the containers and tailing `/mnt/test/log` file - the original PVC is being written to all the time, while the clone isn't.

## Prerequisites

- storage class `example-sc` 
- snapshot storage class `example-snapshot-sc`
- LightOS cluster with snapshots and clones support

## Step-by-step guide

Create the volume and a pod which keeps writing to it:

```
kubectl apply -f 01.example-pvc.yaml
kubectl apply -f 02.example-pod.yaml
```

### Clone from snapshot

Create a snapshot manually and clone from that snapshot:

```
kubectl apply -f 03.example-snapshot.yaml
kubectl apply -f 04.example-pvc-from-snapshot.yaml
kubectl apply -f 05.example-pvc-from-snapshot-pod.yaml
```

Tail both pods and see that the snapshot and clone from the snapshot was successful:

```
kubectl exec -it example-pod -- tail -5 /mnt/test/log
kubectl exec -it example-clone-from-snapshot-pod -- tail -5 /mnt/test/log
```
### Clone from volume

When cloning from volume, no need to create a snapshot manually - the CSI driver will create an intermediate snapshot from which it will clone the volume:

```
kubectl apply -f 06.example-pvc-from-pvc.yaml
kubectl apply -f 07.example-pvc-from-pvc-pod.yaml
```

Tail both pods and see that the clone from the volume was successful:

```
kubectl exec -it example-pod -- tail -5 /mnt/test/log
kubectl exec -it example-clone-from-volume-pod -- tail -5 /mnt/test/log
```
