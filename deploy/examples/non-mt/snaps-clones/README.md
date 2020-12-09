# Snapshots and Clones example

Shows the snapshots and clone from snapshots functionality.
First, we create a PVC and a pod which attaches to it and keeps writing the date to `/mnt/test/log`.
Then, we take a snapshot of the PVC, create a clone PVC using the snapshot as source, and start another pod which sleeps forever.
We can see that the snapshotting and cloning from snapshot succeeded by attaching to each of the containers and tailing `/mnt/test/log` file - the original PVC is being written to all the time, while the clone isn't.

## Prerequisites

- storage class `example-sc` 
- snapshot storage class `example-snapshot-sc`
- LightOS cluster with snapshots and clones support

## Step-by-step guide

```
kubectl apply -f 01.example-pvc.yaml
kubectl apply -f 02.example-pod.yaml
kubectl apply -f 03.example-snapshot.yaml
kubectl apply -f 04.example-pvc-from-snapshot.yaml
kubectl apply -f 05.example-pvc-from-snapshot-pod.yaml
kubectl exec -it example-pod -- tail -5 /mnt/test/log
kubectl exec -it example-pvc-from-snapshot-pod -- tail -5 /mnt/test/log
```
