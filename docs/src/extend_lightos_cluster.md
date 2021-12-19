<div style="page-break-after: always;"></div>

# Overview

The lb-csi-plugin is stateless and holds no persistent information between operations.

Kubernetes API calls that invoke the lb-csi (use LightOS storage), require information of the LightOS management endpoints so the plugin can access the LightOS cluster.

The way to update this management endpoint list to the plugin today is via the `StorageClass.Parameters.mgmt-endpoint` field.

The CSI API does not pass the request. Parameters in some types of API calls to the plugin, This results in situations where the plugin needs to access LightOS API but does not have the required information to do so.

In order to overcome this limitation we defined an internal resource named `ResourceID` which holds this information as the resource ID passed to K8s.

Such `ResourceID` is utilized for representing the `PersistentVolume.spec.volumeHandle` and `VolumeSnapshotContent.status.snapshotHandle` resources.

`ResourceID` is guarantied to pass on every API call from CSI - meaning that we can use it to hold limited state.

`ResourceID` format is: `mgmt:<host>:<port>[,<host>:<port>...]|nguid:<nguid>|proj:<proj>|scheme:<grpc|grpcs>`

When the LightOS cluster is expanded (i.e., adding/changing servers to an existing cluster), the management-endpoints list is updated, 
ensuring that the lb-csi-plugin can access any of the LightOS api-servers.

Since the `PersistentVolume.spec.volumeHandle` and `VolumeSnapshotContent.status.snapshotHandle` fields are immutable, and can't change them post resource creation.

To mitigate this problem, we provide a one-shot script that accesses a K8s cluster via standard kubectl calls, and patches the following resource with the updated information:

* `StorageClass` - will modify the `parameters.mgmt-endpoint` field with new endpoint list.
* `PersistentVolume` - will modify the resource `spec.volumeHandle` by using the `kubectl replace` call.
* `VolumeSnapshotContent` - will replace the resource's `spec.source.volumeHandle` with the new updated `spec.source.snapshotHandle` which will contain the new endpoint list.

This behavior of relying on 'ResourceID' will be fixed in future versions of `lb-csi-plugin`, once the LightOS cluster supports VIP and
all resources will point to a single endpoint, that will not change during LightOS cluster updates.

This script should be idempotent, and should be safe to run on resources that were already updated.

## Usage

below is the patcher help output we provide to patch the existing resources in the cluster.

```bash
lightos-patcher.sh --help

Usage: lightos-patcher.sh [-s <storage_class>] [-e <endpoints>] [-d <backup_directory>]
-v <storage_class>              name of the storage class and all related PVs to update
-s <snapshot_storage_class>     name of the snapshot storage class and all related SnapshotContents to update
-e <endpoints>                  new endpoint list in the form of: <host:port>,<host:port>,...
-d <backup_directory>           folder to backup before and after resources
Examples:
    
    Suppose we have LightOS Cluster los1 with the following mgmt-endpoints:
    192.168.17.2:443,192.168.18.3:443,192.168.20.4:443
    
    After extending this cluster by adding a new server (192.168.20.5:443) we will have the following mgmt-endpoints:
    192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443

    # patch example-sc StorageClass and all PVs related to that StorageClass
    ./lightos-patcher.sh -v example-sc -e 192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443 -d ~/backup
    
    # patch example-sc VolumeSnapshotClass and all VolumeSnapshotContents related to that class
    ./lightos-patcher.sh -s example-snap-sc -e 192.168.17.2:443,192.168.18.3:443,192.168.20.4:443,192.168.20.5:443 -d ~/backup
```

The Order of the commands should be:

1. Apply the script against all `StorageClass`s with the `-v` option. Verify that all StorageClass and PVs are updated.
2. If there are VolumeSnapshots on the cluster, apply the script with the `-s` option.

> **NOTE:**
>
> Avoid operations that might access PV,PVC,VolumeSnapshots resources while running this script
> Operations like replace will delete and recreate the resource with different values. As a result temporarily
> the StorageClass might not be accessible.

On a cluster that has existing PVs before expanding LightOS cluster, run the following:

```bash
./lightos-patcher.sh -v <storage-class-name> -e <new-comma-separated-endpoint-list> -d <backup-folder>
```

This command will:

1. Patch the `StorageClass.Parameters.mgmt-endpoint` with the `new-comma-separated-endpoint-list` value.
2. Look up all `PV`s in the `StorageClass`, and will patch `PersistentVolume.spec.volumeHandle` value with the `new-comma-separated-endpoint-list`.

On a cluster that has `VolumeSnapshot`s created before expanding the LightOS cluster, run the following:

```bash
./lightos-patcher.sh -s <storage-class-name> -e <new-comma-separated-endpoint-list> -d <backup-folder>
```

This command will:

1. Look up all `VolumeSnapshotContent`s in this `VolumeSnapshotClass` and will replace the `VolumeSnapshotContent.spec.source.volumeHandle` value with the `VolumeSnapshotContent.spec.source.snapshotHandle`.

> NOTE:
>
> The `VolumeSnapshotConten.Status.restoreSize` field will be zeroed out because this is a calculated property using `ListSnapshots` - which is not implemented and cannot be modified via API.
>
> This field still remains valid under `VolumeSnapshot.Status.restoreSize`, and if you want to try to restore this snapshot into a PVC with smaller size it will fail
> with the following error:
>
> ```bash
> requested volume size 1073741824 is less than the size 2147483648 for the source snapshot s1
> ```
> 
> as expected.