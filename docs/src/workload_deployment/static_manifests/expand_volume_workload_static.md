## Expand Volume

In order to expand the PV from the “Filesystem VolumeMode Example” to, e.g., 116 GB, the PVC definition needs to be "patched" accordingly:

```bash
kubectl patch pvc example-fs-pvc -p '{"spec":{"resources":{"requests":{"storage": "116Gi" }}}}'
```

Verify that the resize took place, run the following command:

```bash
kubectl describe pvc example-fs-pvc
```
