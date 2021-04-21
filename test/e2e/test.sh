#!/bin/bash
# Copyright (C) 2020 Intel Corporation
# SPDX-License-Identifier: Apache-2.0

SCRIPTPATH="$( cd "$(dirname "$0")" >/dev/null 2>&1 ; pwd -P )"

dbg() {
    if [ "$VERBOSE" = "true" ]; then echo "$(basename "$0"): $*"; fi
}

status() {
    printf '\033[1;35m%s\033[0m\n' "$*"
}

err() {
    printf '\033[1;31m%s\033[0m\n' "$*"
}

info() {
    printf '\033[1;35m%s: %s\033[0m\n' "$(basename "$0")" "$*"
}

success() {
    printf '\033[1;32m%s\033[0m\n' "$*"
}

run() {
    echo "$*"
    "$@" || exit $?
}

cleanup() {
    info "Delete default storage class and snapshot class"
    KUBECONFIG="/root/.kube/config" kubectl delete -f $TESTDIR/storage-class.yaml
    KUBECONFIG="/root/.kube/config" kubectl delete -f $TESTDIR/snapshot-class.yaml
}

test() {
    local focus
    local cmd

    start_time=`date +%s.%N`
    # https://github.com/kubernetes/kubernetes/issues/97993
    info "Create default storage class"
    trap cleanup EXIT
    KUBECONFIG="/root/.kube/config" kubectl create -f $TESTDIR/storage-class.yaml
    KUBECONFIG="/root/.kube/config" kubectl create -f $TESTDIR/snapshot-class.yaml

    info "Start $TEST tests"
    case "$TEST" in
            volume-expand) focus='csi.lightbitslabs.com.*volume-expand' ;;
            volume-expand-block) focus='csi.lightbitslabs.com.*(block volmode).*volume-expand' ;;
            volume-expand-fs) focus='csi.lightbitslabs.com.*fs.*volume-expand' ;;
            volume-mode) focus='csi.lightbitslabs.com.*volumeMode' ;;
            volumes) focus='csi.lightbitslabs.com.*volumes' ;;
            volume-stress) focus='csi.lightbitslabs.com.*volume-stress' ;;
            multi-volume) focus='csi.lightbitslabs.com.*multiVolume' ;;
            provisioning) focus='csi.lightbitslabs.com.*provisioning' ;;
            snapshottable) focus='csi.lightbitslabs.com.*snapshottable' ;;
            snapshottable-stress) focus='csi.lightbitslabs.com.*snapshottable-stress' ;;
            capacity) focus='csi.lightbitslabs.com.*capacity' ;;
            disruptive) focus='csi.lightbitslabs.com.*disruptive' ;;
            ephemeral) focus='csi.lightbitslabs.com.*ephemeral' ;;
            volumeIO) focus='csi.lightbitslabs.com.*volumeIO' ;;
        *) focus='csi.lightbitslabs.com' ;;
    esac

    # we comment the redirect to test.log cause it hides the error code of the command
    # since we run this under systemd we have the journald log so no need to redirect
    #cmd="KUBECONFIG=\"$KUBECONFIG\" ./e2e.test -ginkgo.v -ginkgo.skip='Disruptive' -ginkgo.reportPassed -ginkgo.focus='${focus}' -storage.testdriver=\"$TESTDIR\"/test-driver.yaml -report-dir=\"$TESTDIR\" | tee $LOGSDIR/test.log"
    cmd="KUBECONFIG=\"$KUBECONFIG\" ./e2e.test -ginkgo.v -ginkgo.skip='Disruptive' -ginkgo.reportPassed -ginkgo.focus='${focus}' -storage.testdriver=\"$TESTDIR\"/test-driver.yaml -report-dir=\"$TESTDIR\""

    pushd $TESTDIR
    info "Start '$TEST' tests"
    info "$cmd"

    eval $cmd
    result=`echo ${PIPESTATUS[0]}`
    # we comment the redirect to test.log cause it hides the error code of the command
    # since we run this under systemd we have the journald log so no need to redirect
    # if [[ $result -eq 0 ]]; then
    #     sed -i -r 's/\x1B\[([0-9]{1,3}(;[0-9]{1,2})?)?[mGK]//g' "$LOGSDIR"/test.log
    # fi
    popd

    end_time=`date +%s.%N`
    runtime=$( echo "$end_time - $start_time" | bc -l )
    info "=============================="
    info "Done. Took: $runtime seconds"
    info "=============================="
    exit $result
}

download() {
    local test_archive_url="https://dl.k8s.io/$CLUSTER_VERSION/kubernetes-test-linux-amd64.tar.gz"
    
    mkdir -p $TESTDIR
    [ -e "$TESTDIR"/e2e.test -a -e "$TESTDIR"/ginkgo ] && {
        dbg "Found e2e.test and ginkgo in $TESTDIR, skip download"
        return 0
    }

    info "Downloading kubernetes/test/bin/e2e.test and kubernetes/test/bin/ginkgo from $test_archive_url to $TESTDIR"
    (cd $TESTDIR && curl -s --location "$test_archive_url" | tar --strip-components=3 -zxf - kubernetes/test/bin/e2e.test kubernetes/test/bin/ginkgo)
    info "Done"
}

generate() {
    cat > $TESTDIR/storage-class.yaml <<EOF
kind: StorageClass
apiVersion: storage.k8s.io/v1
metadata:
  name: lb-csi-sc
  annotations:
    storageclass.kubernetes.io/is-default-class: "true"
provisioner: csi.lightbitslabs.com
parameters:
  mgmt-endpoint: $MGMT_ENDPOINT
  replica-count: "$REPLICA_COUNT"
  compression: $COMPRESSION
  mgmt-scheme: $MGMT_SCHEME
  project-name: $PROJECT_NAME
  csi.storage.k8s.io/controller-publish-secret-name: $SECRET_NAME
  csi.storage.k8s.io/controller-publish-secret-namespace: $SECRET_NAMESPACE
  csi.storage.k8s.io/node-stage-secret-name: $SECRET_NAME
  csi.storage.k8s.io/node-stage-secret-namespace: $SECRET_NAMESPACE
  csi.storage.k8s.io/node-publish-secret-name: $SECRET_NAME
  csi.storage.k8s.io/node-publish-secret-namespace: $SECRET_NAMESPACE
  csi.storage.k8s.io/provisioner-secret-name: $SECRET_NAME
  csi.storage.k8s.io/provisioner-secret-namespace: $SECRET_NAMESPACE
  csi.storage.k8s.io/controller-expand-secret-name: $SECRET_NAME
  csi.storage.k8s.io/controller-expand-secret-namespace: $SECRET_NAMESPACE
EOF

    cat > $TESTDIR/snapshot-class.yaml <<EOF
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: lb-csi-snapshot-sc
driver: csi.lightbitslabs.com
deletionPolicy: Delete
parameters:
  csi.storage.k8s.io/snapshotter-secret-name: $SECRET_NAME
  csi.storage.k8s.io/snapshotter-secret-namespace: $SECRET_NAMESPACE
  csi.storage.k8s.io/snapshotter-list-secret-name: $SECRET_NAME
  csi.storage.k8s.io/snapshotter-list-secret-namespace: $SECRET_NAMESPACE
  snapshot.storage.kubernetes.io/deletion-secret-name: $SECRET_NAME
  snapshot.storage.kubernetes.io/deletion-secret-namespace: $SECRET_NAMESPACE
EOF

    cat > $TESTDIR/test-driver.yaml <<EOF
StorageClass:
  FromName: false
  FromFile: $TESTDIR/storage-class.yaml
SnapshotClass:
  FromName: false
  FromFile: $TESTDIR/snapshot-class.yaml
DriverInfo:
  Name: csi.lightbitslabs.com
  SupportedSizeRange:
    Min: 1Gi
  StressTestOptions:
    # Number of pods to create in the test. This may also create up to 1 volume per pod.
    NumPods: 2
    # Number of times to restart each Pod.
    NumRestarts: 2
  VolumeSnapshotStressTestOptions:
    # Number of pods to create in the test. This may also create up to 1 volume per pod.
    NumPods: 2
    # Number of snapshots to create for each volume.
    NumSnapshots: 2
  Capabilities:
    persistence: true
    exec: true
    controllerExpansion: true
    nodeExpansion: true
    block: true
    snapshotDataSource: true
    pvcDataSource: true
  # Volume provisioning metrics are compared to a high baseline. Failure to pass would suggest a performance regression.
  PerformanceTestOptions:
      ProvisioningOptions:
      VolumeSize: "1Gi"
      Count: 3
      ExpectedMetrics:
          AvgLatency: 1m
          Throughput: 0.5
EOF
    dbg "Generated $TESTDIR/storage-class.yaml:"
    [ "$VERBOSE" = "true" ] && cat $TESTDIR/storage-class.yaml
    dbg "Generated $TESTDIR/snapshot-class.yaml:"
    [ "$VERBOSE" = "true" ] && cat $TESTDIR/snapshot-class.yaml
    dbg "Generated $TESTDIR/test-driver.yaml:"
    [ "$VERBOSE" = "true" ] && cat $TESTDIR/test-driver.yaml
}

usage() {
    echo "usage: $(basename "${0}") [-hv] [-k path_to_kubeconfig] [-m mgmt-endpoint] [-r replica-count] [-c enable-compression] [-s|--skip-test] [-n secret-name] [-N secret-namespace]"
    echo "  options:"
    echo "      -h|--help - show this help menu"
    echo "      -v|--verbose - verbosity on"
    echo "      -k|--kubeconfig - path to kubeconfig, default is KUBECONFIG environment variable"
    echo "      -m|--mgmt-endpoint - mgmt-endpoint parameter for the storage class. Default is using LB_CSI_SC_MGMT_ENDPOINT environment variable"
    echo "      -S|--mgmt-scheme - mgmt-scheme parameter for the storage class. options: [grpc, grpcs] (default: grpcs)"
    echo "      -r|--replica-count - replica-count parameter for the storage class. Default is using LB_CSI_SC_REPLICA_COUNT environment variable OR 3 if not defined"
    echo "      -c|--enable-compression - compression parameter for the storage class. Default is using LB_CSI_SC_COMPRESSION environment variable OR disabled if not defined"
    echo "      -s|--skip-test - only download and generate the test files without running the test"
    echo "      -t|--test - run a specific test. Supported tests - ${SUPPORTED_TESTS[@]} (default all)"
    echo "      -d|--test-dir - test path, where all artifacts will be stored. (default $TESTDIR)"
    echo "      -l|--logs-dir - logs path, where test.log will be stored. (default $TESTDIR)"
    echo "      -p|--project-name - LightOS project-name to use. (default 'default')"
    echo "      -n|--secret-name - Secret name to use. (required)"
    echo "      -N|--secret-namespace - namespace that the secret resides. (default 'default')"
}

main() {
    if ! OPTS=$(getopt -o 'hvk:m:S:r:cst:d:l:n:N:p:' --long help,verbose,kubeconfig:,mgmt-endpoint:,mgmt-scheme:,replica-count:,compression,skip-test,test:,test-dir:,logs-dir:secret-name:secret-namespace:project-name: -n 'parse-options' -- "$@"); then
        err "Failed parsing options." >&2 ; usage; exit 1 ;
    fi

    eval set -- "$OPTS"

    while true; do
        case "$1" in
            -v | --verbose)             VERBOSE=true; shift ;;
            -h | --help)                usage; exit 0; shift ;;
            -k | --kubeconfig)          KUBECONFIG="$2"; shift; shift ;;
            -m | --mgmt-endpoint)       MGMT_ENDPOINT="$2"; shift; shift ;;
            -S | --mgmt-scheme)         MGMT_SCHEME="$2"; shift; shift ;;
            -r | --replica-count)       REPLICA_COUNT="$2"; shift; shift ;;
            -c | --compression)         COMPRESSION=enabled; shift ;;
            -s | --skip-test)           SKIP_TEST=true; shift ;;
            -t | --test)                TEST="$2"; shift; shift ;;
            -d | --test-dir)            TESTDIR="$(readlink -f $2)"; shift; shift ;;
            -l | --logs-dir)            LOGSDIR="$(readlink -f $2)"; shift; shift ;;
            -n | --secret-name)         SECRET_NAME="$2"; shift; shift ;;
            -N | --secret-namespace)    SECRET_NAMESPACE="$2"; shift; shift ;;
            -p | --project-name)        PROJECT_NAME="$2"; shift; shift ;;
            -- ) shift; break ;;
            * ) err "unsupported argument $1"; usage; exit 1 ;;
        esac
    done

    if [ -z "$KUBECONFIG" ]; then
        err "KUBECONFIG environment variable not set and -k|--kubeconfig option not used"
        usage
        exit 1
    fi

    if [ -z "$MGMT_ENDPOINT" ]; then
        err "LB_CSI_SC_MGMT_ENDPOINT environment variable not set and -m|--mgmt-endpoint variable not used"
        usage
        exit 1
    fi

    if [ -z "$SECRET_NAME" ]; then
        err "SECRET_NAME environment variable not set and -n|--secret-name variable not used"
        usage
        exit 1
    fi

    if [ -z "$SECRET_NAMESPACE" ]; then
        info "SECRET_NAMESPACE environment variable not set and -N|--secret-namespace using default"
        SECRET_NAMESPACE="default"
    fi

    if [ -z "$PROJECT_NAME" ]; then
        info "PROJECT_NAME environment variable not set and -p|--project-name using default"
        PROJECT_NAME="default"
    fi

    if [ -z "$MGMT_SCHEME" ]; then
        MGMT_SCHEME="grpcs"
    fi

    CLUSTER_VERSION=$(kubectl --kubeconfig $KUBECONFIG version --short | grep Server | awk '{print $3}')
    if [ -z "$CLUSTER_VERSION" ]; then
        err "Failed to get k8s cluster version using kubectl --kubeconfig $KUBECONFIG version --short"
        usage
        exit 1
    fi

    if [[ ! " ${SUPPORTED_TESTS[@]} " =~ " ${TEST} " ]]; then
        err "Unsupported test ${TEST}"
        usage
        exit 1
    fi
    [ -z "$TESTDIR" ] && TESTDIR="$SCRIPTPATH"/results/"$CLUSTER_VERSION"/"$TEST"
    [ -z "$LOGSDIR" ] && LOGSDIR="$TESTDIR"

    dbg "TEST=$TEST"
    dbg "TESTDIR=$TESTDIR"
    dbg "LOGSDIR=$LOGSDIR"
    dbg "KUBECONFIG=$KUBECONFIG"
    dbg "CLUSTER_VERSION=$CLUSTER_VERSION"
    dbg "MGMT_ENDPOINT=$MGMT_ENDPOINT"
    dbg "MGMT_SCHEME=$MGMT_SCHEME"
    dbg "REPLICA_COUNT=$REPLICA_COUNT"
    dbg "COMPRESSION=$COMPRESSION"
    dbg "SECRET_NAME=$SECRET_NAME"
    dbg "SECRET_NAMESPACE=$SECRET_NAMESPACE"
    dbg "PROJECT_NAME=$PROJECT_NAME"

    download
    generate
    [ "$SKIP_TEST" = "false" ] && test
}

VERBOSE=false
CLUSTER_VERSION=
TESTDIR=
LOGSDIR=
SKIP_TEST=false
SUPPORTED_TESTS=(all volume-expand volume-expand-block volume-expand-fs provisioning multi-volume volumes volume-mode snapshottable snapshottable-stress volumeIO capacity disruptive ephemeral volume-stress)
TEST=all
# storage class lb specific params
MGMT_ENDPOINT="$LB_CSI_SC_MGMT_ENDPOINT"
REPLICA_COUNT="${LB_CSI_SC_REPLICA_COUNT-3}"
COMPRESSION="${LB_CSI_SC_COMPRESSION-disabled}"

main "$@"