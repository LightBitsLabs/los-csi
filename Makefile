# Copyright (C) 2016--2020 Lightbits Labs Ltd.
# SPDX-License-Identifier: Apache-2.0

.PHONY: all test test_long build build-image push clean

ifeq ($(V),1)
    GO_VERBOSE := -v
endif

TTY=$(if $(shell [ -t 0 ] && echo 1),-it, )

# do NOT change or force these from the cmd-line for custom builds, use
# $PLUGIN_NAME/$PLUGIN_VER for that instead:
override BIN_NAME := lb-csi-plugin
override DEFAULT_REL := 0.0.0
override VERSION_RELEASE := $(or $(shell cat VERSION 2>/dev/null),$(DEFAULT_REL))
override RELEASE := $(if $(BUILD_ID),$(VERSION_RELEASE).$(BUILD_ID),$(VERSION_RELEASE))

# pass in $SIDECAR_DOCKER_REGISTRY to use a local Docker image cache:
SIDECAR_DOCKER_REGISTRY := $(or $(SIDECAR_DOCKER_REGISTRY),registry.k8s.io)

# these vars are sometimes passed in from the outside:
#   $BUILD_HASH

# for local testing you can override those and $DOCKER_REGISTRY:
override PLUGIN_NAME := $(or $(PLUGIN_NAME),$(BIN_NAME))
override PLUGIN_VER := $(or $(PLUGIN_VER),$(RELEASE))

DISCOVERY_CLIENT_DOCKER_TAG := lb-nvme-discovery-client:$(or $(DISCOVERY_CLIENT_BUILD_HASH),$(RELEASE))

PKG_PREFIX := github.com/lightbitslabs/los-csi

override BUILD_HOST := $(or $(BUILD_HOST),$(shell hostname))
override BUILD_TIME := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
override GIT_VER := $(or $(GIT_VER), $(or \
    $(shell git describe --tags --abbrev=8 --always --long --dirty),UNKNOWN))


# set BUILD_HASH to GIT_VER if not provided
override BUILD_HASH := $(or $(BUILD_HASH),$(GIT_VER))
TAG := $(if $(BUILD_ID),$(PLUGIN_VER),$(BUILD_HASH))
DOCKER_TAG := $(PLUGIN_NAME):$(TAG)


LDFLAGS ?= \
    -X $(PKG_PREFIX)/pkg/driver.version=$(PLUGIN_VER) \
    -X $(PKG_PREFIX)/pkg/driver.versionGitCommit=$(GIT_VER) \
    $(and $(BUILD_HASH), -X $(PKG_PREFIX)/pkg/driver.versionBuildHash=$(BUILD_HASH)) \
    $(and $(BUILD_ID), -X $(PKG_PREFIX)/pkg/driver.versionBuildID=$(BUILD_ID)) \
    -extldflags "-static"
override GO_VARS := CGO_ENABLED=0

override LABELS := \
	--label org.opencontainers.image.title="Lightbits CSI Plugin" \
	--label org.opencontainers.image.version="$(PLUGIN_VER)" \
	--label org.opencontainers.image.description="CSI plugin for Lightbits Cluster" \
	--label org.opencontainers.image.authors="Lightbits Labs <support@lightbitslabs.com>" \
	--label org.opencontainers.image.documentation="https://www.lightbitslabs.com/support/" \
	--label org.opencontainers.image.revision=$(GIT_VER) \
	--label org.opencontainers.image.created=$(BUILD_TIME) \
	$(and $(BUILD_HASH), --label version.lb-csi.hash="$(BUILD_HASH)") \
	$(if $(BUILD_HASH),, --label version.lb-csi.build.host="$(BUILD_HOST)") \
	$(if $(BUILD_ID), --label version.lb-csi.build.id=$(BUILD_ID),)

print-% : ## print the variable name to stdout
	@echo $($*)

YAML_PATH := deploy/k8s

IMG_BUILDER := image-builder:v0.0.1
IMG := $(DOCKER_REGISTRY)/$(DOCKER_TAG)

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)



# NOTE: some tests have additional external dependencies (e.g. network access,
# presence of remote LightOS cluster. these will not be run by default and require
# specific build tags to be enabled (as well as additional cmd-line params in some cases),
# e.g.: have_net, have_lb. see specific tests for details.
# you'll also want to run these tests with:
#     go test <whatever> -count=1 <whatever>
# to make sure they're actually being run against an external entity, rather
# than `go test` just regurgitating cached old test results.
#
# TODO: consider adding a separate target 'lint' to push it through the entire 'gometalinter' (or,
# preferably, 'golangci-lint'!) with custom config - but that implies quite a
# bit of external dependencies as part of the toolchain...
test: ## Run short test suite
	$(GO_VARS) go test $(GO_VERBOSE) -short -cover ./...

test_long: ## Run long test suite (you're looking at over 10min here...)
	$(GO_VARS) go test $(GO_VERBOSE) -cover ./...

fmt: ## Run go fmt against code
	go fmt ./...

vet: ## Run go vet against code.
	go vet ./...

build: ## Build plugin binary.
	$(GO_VARS) go build $(GO_VERBOSE) -a -ldflags '$(LDFLAGS)' -o deploy/$(BIN_NAME)

deploy/k8s:
	mkdir -p deploy/k8s

manifests: lb-csi-manifests snapshot-controller-manifests

snapshot-controller-manifests: verify_image_registry deploy/k8s
	helm template deploy/helm/snapshot-controller-3/ \
	    --include-crds \
		--namespace=kube-system \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) > deploy/k8s/snapshot-controller-3.yaml
	helm template deploy/helm/snapshot-controller-4/ \
	    --include-crds \
		--namespace=kube-system \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) > deploy/k8s/snapshot-controller-4.yaml

lb-csi-manifests: verify_image_registry deploy/k8s
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.17 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.17.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.17 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.17-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.18 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.18.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.18 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.18-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.19 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.19.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.19 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.19-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.20 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.20.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.20 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.20-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.21 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.21.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.21 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.21-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.22 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.22.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.22 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.22-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.23 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.23.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.23 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.23-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.24 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.24.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.24 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.24-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.25 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.25.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.25 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.25-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.26 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.26.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.26 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.26-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.27 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.27.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.27 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.27-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.28 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.28.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.28 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.28-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.29 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.29.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.29 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.29-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.30 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.30.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.30 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.30-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.32 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.32.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.32 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.32-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.33 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.33.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.33 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.33-dc.yaml


deploy/examples:
	mkdir -p deploy/examples

examples_manifests: deploy/examples
	helm template --set storageclass.enabled=true \
		--set global.storageClass.mgmtEndpoints="10.10.0.2:443\,10.10.0.3:443\,10.10.0.4:443" \
		--set global.jwtSecret.jwt="eyJhbGciOiJSUzI1NiIsImtpZCI6InN5c3RlbTpyb290IiwidHlwIjoiSldUIn0.eyJhdWQiOiJMaWdodE9TIiwiZXhwIjoxNjY0MTc4NjA5LCJpYXQiOjE2MzI2NDI2MDksImlzcyI6InN5c3Rlc3RzIiwianRpIjoiU0gwLXhVR1R3M2hLdFFRRURzckVtUSIsIm5iZiI6MTYzMjY0MjYwOSwicm9sZXMiOlsic3lzdGVtOmNsdXN0ZXItYWRtaW4iXSwic3ViIjoibGlnaHRvcy1jbGllbnQifQ.qd1L0FnFIYwPZuZY0D2109l1a8D6YekriiPLnNNhu7MygrMNQFZ9hbkv1XNIeZR0mopTFmziMYV95TzP7fEh2_4Y4Q9rmpoi-d2NCsK3dwGdI1DIhjbC07YZUOmg0nNPcJFeWvbFv-gPaIkKpOBY9sL8tNPLVc3RsazksfOd4xC6InGG509sfoPDzIBW84WahezEuma32Ljw4BWDBAK-IQ3UOEEvpiOh-YkEKeLGkQNNPNqRUoEEnwTs5Vue9DC0L9OqRIfK1K4GEGOlF1P69jJn1tdRoUf8z1fZpvdt4GBhR5L8pK7gSQpDxXI-OgXi0YYvZE-2IIcGJjXXHa5OiA" \
		deploy/helm/lb-csi-workload-examples > deploy/examples/secret-and-storage-class.yaml
	helm template --set block.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/block-workload.yaml
	helm template --set filesystem.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/filesystem-workload.yaml
	helm template --set statefulset.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/statefulset-workload.yaml
	helm template --set preprovisioned.enabled=true \
		--set global.storageClass.mgmtEndpoints="10.10.0.2:443\,10.10.0.3:443\,10.10.0.4:443" \
		--set global.jwtSecret.name="example-secret" \
		--set global.jwtSecret.namespace="default" \
		--set preprovisioned.lightosVolNguid=60907a32-76c7-11eb-ac25-fb55927189f9 \
		--set preprovisioned.volumeMode=Filesystem \
		--set preprovisioned.storage=1Gi \
		deploy/helm/lb-csi-workload-examples > deploy/examples/preprovisioned-filesystem-workload.yaml
	helm template --set preprovisioned.enabled=true \
		--set global.storageClass.mgmtEndpoints="10.10.0.2:443\,10.10.0.3:443\,10.10.0.4:443" \
		--set global.jwtSecret.name="example-secret" \
		--set global.jwtSecret.namespace="default" \
		--set preprovisioned.lightosVolNguid=60907a32-76c7-11eb-ac25-fb55927189f9 \
		--set preprovisioned.volumeMode=Block \
		--set preprovisioned.storage=1Gi \
		deploy/helm/lb-csi-workload-examples > deploy/examples/preprovisioned-block-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=v1.24.0 \
		--set snaps.stage=snapshot-class \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-example-snapshot-class.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=v1.24.0 \
		--set snaps.stage=example-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-example-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=v1.24.0 \
		--set snaps.stage=snapshot-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-snapshot-from-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=v1.24.0 \
		--set snaps.stage=pvc-from-snapshot \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-snapshot-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=v1.24.0 \
		--set snaps.stage=pvc-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-pvc-workload.yaml

verify_image_registry:
	@if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't push" ; exit 1 ; fi

build-image: verify_image_registry build  ## Builds the image, but does not push.
	@docker build $(LABELS) -t $(IMG) deploy

push: verify_image_registry ## Push it to registry specified by DOCKER_REGISTRY variable
	@docker push $(IMG)

clean:
	@$(GO_VARS) go clean $(GO_VERBOSE)
	@rm -rf deploy/$(BIN_NAME) $(YAML_PATH)/*.yaml \
		deploy/*.rpm *~ deploy/*~ build/* \
		deploy/helm/charts/* deploy/k8s \
		deploy/examples \
		docs/book
	@git clean -f '*.orig'

image_tag: ## Print image tag
	@echo $(DOCKER_TAG)

full_image_tag: verify_image_registry ## Prints full name of plugin image.
	@echo $(IMG)

bundle: verify_image_registry manifests examples_manifests helm_package
	@mkdir -p ./build
	rm -rf build/lb-csi-bundle-*.tar.gz
	@if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't generate bundle" ; exit 1 ; fi
	@tar -C deploy -czvf build/lb-csi-bundle-$(RELEASE).tar.gz \
		k8s examples helm/charts lightos-patcher

deploy/helm/charts:
	mkdir -p deploy/helm/charts

helm_package: deploy/helm/charts
	rm -rf ./deploy/helm/charts/*
	helm package -d ./deploy/helm/charts deploy/helm/lb-csi
	helm lint ./deploy/helm/charts/lb-csi-plugin-*.tgz
	helm package -d ./deploy/helm/charts deploy/helm/lb-csi-workload-examples
	helm lint ./deploy/helm/charts/lb-csi-workload-examples-*.tgz
	helm package -d ./deploy/helm/charts deploy/helm/snapshot-controller-3
	helm lint ./deploy/helm/charts/snapshot-controller-3-*.tgz
	helm package -d ./deploy/helm/charts deploy/helm/snapshot-controller-4
	helm lint ./deploy/helm/charts/snapshot-controller-4-*.tgz

helm_package_upload: helm_package
	@$(BUILD_FLAGS) ./scripts/upload-helm-packages.sh

image-builder: ## Build image for building the plugin and the bundle.
	@docker build \
		--build-arg UID=$(shell id -u) \
		--build-arg GID=$(shell id -g) \
		--build-arg DOCKER_GID=$(shell getent group docker | cut -d: -f3) \
		-t ${IMG_BUILDER} -f Dockerfile.builder .

docker-cmd := docker run --rm --privileged $(TTY) \
		-e DOCKER_REGISTRY=$(DOCKER_REGISTRY) \
		-e SIDECAR_DOCKER_REGISTRY=$(SIDECAR_DOCKER_REGISTRY) \
		-e BUILD_HASH=$(BUILD_HASH) \
		-e GIT_VER=$(GIT_VER) \
		-e BUILD_ID=$(BUILD_ID) \
		-e RELEASE=$(RELEASE) \
		-e PLUGIN_VER=$(PLUGIN_VER) \
		-e HELM_CHART_REPOSITORY=$(HELM_CHART_REPOSITORY) \
		-e HELM_CHART_REPOSITORY_USERNAME=$(HELM_CHART_REPOSITORY_USERNAME) \
		-e HELM_CHART_REPOSITORY_PASSWORD=$(HELM_CHART_REPOSITORY_PASSWORD) \
		-e DISCOVERY_CLIENT_BUILD_HASH=$(DISCOVERY_CLIENT_BUILD_HASH) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v `pwd`:/go/src/$(PKG_PREFIX) \
		-w /go/src/$(PKG_PREFIX) \
		-h $(BUILD_HOST) \
		${IMG_BUILDER}

docker-run: image-builder ## Enter image-builder shell.
	@${docker-cmd} sh

docker-helm-package: image-builder ## Generate helm packages in image-builder.
	@${docker-cmd} sh -c "$(MAKE) helm_package"

docker-helm-package-upload: image-builder ## Upload helm packages to Helm Repo in image-builder.
	@${docker-cmd} sh -c "$(MAKE) helm_package_upload"

docker-build: image-builder ## Build plugin and package it in image-builder.
	@${docker-cmd} sh -c "$(MAKE) build-image"

docker-push: push

docker-bundle: image-builder ## Generate manifests for plugin deployment and example manifests as well as helm packages in image-builder
	@${docker-cmd} sh -c "$(MAKE) bundle"

docker-test: image-builder ## Run short test suite in image-builder
	${docker-cmd} sh -c "$(MAKE) test"

.PHONY: docs
docs:
	@$(BUILD_FLAGS) $(MAKE) -f docs/Makefile.docs pandoc-pdf
