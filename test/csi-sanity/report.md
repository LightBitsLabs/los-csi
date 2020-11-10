# results

csi-sanity

|                                                         Test                                                         |Result|
|----------------------------------------------------------------------------------------------------------------------|------|
|Node Service should be idempotent                                                                                     |FAIL  |
|Controller Service [Controller Server] ValidateVolumeCapabilities should fail when the requested volume does not exist|FAIL  |
|Controller Service [Controller Server] ControllerPublishVolume should fail when the volume does not exist             |FAIL  |
|Controller Service [Controller Server] ControllerPublishVolume should fail when the node does not exist               |FAIL  |
|Controller Service [Controller Server] volume lifecycle should be idempotent                                          |FAIL  |
|ExpandVolume [Controller Server] should work                                                                          |FAIL  |
