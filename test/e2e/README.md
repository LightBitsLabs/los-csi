# e2e testing

Helper script to run e2e testing on kubernetes cluster running Lightbits CSI plugin, based on [Testing of CSI drivers blog post](https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/).

The script downloads the test suite (e2e and ginkgo) from https://dl.k8s.io/<k8s version>/kubernetes-test-linux-amd64.tar.gz, generates a storage-class.yaml and a test-driver.yaml based on either environment variables (see below) or optional parameters to set the Lightbits paramters, then runs the tests.

There is only one mandatory variable which is the mgmt-endpoints which connects the generated storage-class to the Lightbits storage cluster.
Set this variable using either `LB_CSI_SC_MGMT_ENDPOINT` environment variable or pass `-m|--mgmt-endpoint` to the script.

Other optional variables include the replication factor (default 3) and compression (default disabled).

The lightbits CSI plugin currently supports only the following capabilities described in [3]:

 - exec
 - persistence
 - controllerExpansion
 - nodeExpansion

These are hardcoded in the script, once a new capability is supported, this script needs to be updated.

# Running the tests

To run tests: `./test.sh -v --kubeconfig </path/to/config> -m <endpoints> -r <replication> -t <test> -n <secret-name> -N <secret-ns>`

Examples:

Run all tests
```
./test.sh -v --kubeconfig ~/.kube/config -m "10.10.10.51:80,10.10.10.52:80,10.10.10.53:80" -r 3 -t all -n system-admin-secret -N default
```

Run all block tests
```
./test.sh -v --kubeconfig=~/.kube/config -m "10.10.10.51:80,10.10.10.52:80,10.10.10.53:80" -r 3 -t block -n system-admin-secret -N default
```

Run all volume expand tests
```
./test.sh -v --kubeconfig=~/.kube/config -m "10.10.10.51:80,10.10.10.52:80,10.10.10.53:80" -r 3 -t resize -n system-admin-secret -N default
```

# Running specific test suites not covered by the script

By default, the script runs all test suites by providing `csi.lightbitslabs.com` to the `ginkgo.focus` argument.
If you wish to run all testscases from a specific test suite, you should first run the script with the `--skip-test` argument so it will download the test binaries and generate the test yamls.
Then, to run a specific test suite, first find out the name of the suite you want to run from [kubernetes/test/e2e/storage/testsuites/base.go](https://github.com/kubernetes/kubernetes/blob/6d01c5a58996d1619ac049c2b3077274299eb2d0/test/e2e/storage/testsuites/base.go#L76).
For example, the volume expand test suite name is `volume-expand` and defined in `initVolumeExpandTestSuite()` function in [kubernetes/test/e2e/storage/testsuites/volume_expand.go](https://github.com/kubernetes/kubernetes/blob/6d01c5a58996d1619ac049c2b3077274299eb2d0/test/e2e/storage/testsuites/volume_expand.go#L62).

In order to run all tests in that suite, run:

```
KUBECONFIG="$KUBECONFIG" ./e2e.test -ginkgo.v -ginkgo.focus='csi.lightbitslabs.com.*volume-expand' -report-dir=/tmp/ -storage.testdriver=./test-driver.yaml
```

In order to run a specific test case, add it to the ginkgo focus regex. For example in order to run ["should resize volume when PVC is edited while pod is using it"](https://github.com/kubernetes/kubernetes/blob/6d01c5a58996d1619ac049c2b3077274299eb2d0/test/e2e/storage/testsuites/volume_expand.go#L238), run:

```
KUBECONFIG="$KUBECONFIG" ./e2e.test -ginkgo.v -ginkgo.focus='csi.lightbitslabs.com.*should.resize.volume.when.PVC.is.edited.while.pod.is.using.it' -report-dir=/tmp/ -storage.testdriver=./test-driver.yaml
```

# References

[1] https://kubernetes.io/blog/2020/01/08/testing-of-csi-drivers/

[2] https://github.com/kubernetes/kubernetes/blob/master/test/e2e/storage/external/README.md

[3] https://github.com/kubernetes/kubernetes/blob/master/test/e2e/storage/testsuites/testdriver.go

[4] https://github.com/kubernetes/kubernetes/blob/master/test/e2e/storage/external/external.go
