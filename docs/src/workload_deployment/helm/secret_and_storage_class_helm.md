
## Secret and StorageClass Chart

This shart will install the following resources:

- A `Secret` containing the lightos JWT
- A `StorageClass` referencing the secret and configured with all values needed to provision volumes on LightOS.

### Deploy Secret And StorageClasses Workload

```bash
helm install \
  --set storageclass.enabled=true \
  --set global.storageClass.mgmtEndpoints="$MGMT_EP" \
  --set global.jwtSecret.jwt="$LIGHTOS_JWT" \
  lb-csi-workload-examples-sc \
  lightbits-helm-repo/lb-csi-workload-examples
```

Will output:

```bash
NAME: lb-csi-workload-examples-sc
LAST DEPLOYED: Sun Feb 21 16:12:56 2021
NAMESPACE: default
STATUS: deployed
REVISION: 1
TEST SUITE: None
```

Provided is an example Secret created by helm:

  ```yaml
  # Source: lb-csi-workload-examples/templates/secret.yaml
  apiVersion: v1
  kind: Secret
  metadata:
    name: example-secret
    namespace: default
  type: lightbitslabs.com/jwt
  data:
    jwt: |-
      ZXlKaGJHY2lPaUpTVXpJMU5pSXNJbXRwWkNJNkluTjVjM1JsYlRweWIyOTBJaXdpZEhsd0lqb2lT
      bGRVSW4wLmV5SmhkV1FpT2lKTWFXZG9kRTlUSWl3aVpYaHdJam94TmpRMU5UQTNOemcyTENKcFlY
      UWlPakUyTVRNNU56RTNPRFlzSW1semN5STZJbk41YzNSbGMzUnpJaXdpYW5ScElqb2lhWFJ5UjNN
      Mk1sTk1hMmxhY2xKdlNuWjNXazFhZHlJc0ltNWlaaUk2TVRZeE16azNNVGM0Tml3aWNtOXNaWE1p
      T2xzaWMzbHpkR1Z0T21Oc2RYTjBaWEl0WVdSdGFXNGlYU3dpYzNWaUlqb2liR2xuYUhSdmN5MWpi
      R2xsYm5RaWZRLlc5QXMwdTJQZnFudTIzZ3U0YXFYcTBKMXZETUJ6bkVfT3dkZkxGeEgzMUdZZVAx
      WHFqbUNLUWlZS3pJcXlmcTgweTdCZC02azZvZlVXbzlRZ0FDb1J6LUhRWTJjc1pYdHVHTGRpRzN3
      YUF3aEs3QjRIQnhROFAzSnpSeno4TzJLOVg1Z3dRY19xYnpjYTBNaUlrWTZVVjVTOWNEMTROTHNQ
      RExwUjdvOFRMbFozbm9kSDZiRlNNVjlPeF9GRXBvTGVidzRWLUlvaURiTV9NdTFDSzZCOUJGeFpN
      RTV6NmJIMXlkSDZFWnRuUFlRaUVrRVdlUzFHMUJSTVNfR0hGN3Nja2NYU0c3Q1pkSFFqOHY1b0Y1
      YS1USHNVdXR0dmFIc1hUS3FzREFkOHRvbEphZUNUN0NWRFFHX0xUQ1hYZ3dudUI3c0ZRaHJHbHhR
      Mkw3V3BlNzczdw==
  ```

> **NOTICE:**
> 
> The chart will validate that the required fields are provided.
> If they are not provided an error will be presented.
>
> For example, if `global.jwtSecret.jwt` is not provided, we will get the following error:
>
> ```bash
> Error: execution error at (lb-csi-workload-examples/charts/storageclass/templates/secret.yaml:1:85): global.jwtSecret.jwt field is required
> ```
> 

### Verify Secret And StorageClasses Workload

Verify that all resources where created:

```bash
kubectl get sc,secret
NAME                                     PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE   ALLOWVOLUMEEXPANSION   AGE
storageclass.storage.k8s.io/example-sc   csi.lightbitslabs.com   Delete          Immediate           true                   5m27s

NAME                                                       TYPE                                  DATA   AGE
secret/example-secret                                      lightbitslabs.com/jwt                 1      5m27s
```

### Uninstall Secret And StorageClasses Workload

Once done with deployment examples you can delete storageclass resources.

```bash
helm uninstall lb-csi-workload-examples-sc
release "lb-csi-workload-examples-sc" uninstalled
```
