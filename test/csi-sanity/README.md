# csi-sanity testing

Running csi-sanity requires access to LightOS cluster, defined via CSI_SANITY_MGMT_ENDPOINT environment variable.
To run csi-sanity test suite, run `make test` from the root folder (runs all tests + csi-sanity) or `go test` from this directory.
