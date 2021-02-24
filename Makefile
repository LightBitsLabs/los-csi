# Copyright (C) 2016--2020 Lightbits Labs Ltd.
# SPDX-License-Identifier: Apache-2.0

.PHONY: all test test_long build package push clean

ifeq ($(V),1)
    GO_VERBOSE := -v
endif

# do NOT change or force these from the cmd-line for custom builds, use
# $PLUGIN_NAME/$PLUGIN_VER for that instead:
override BIN_NAME := lb-csi-plugin
override DEFAULT_REL := 0.0.0
override VERSION_RELEASE := $(or $(shell cat VERSION 2>/dev/null),$(DEFAULT_REL))
override RELEASE := $(if $(BUILD_ID),$(VERSION_RELEASE).$(BUILD_ID),$(VERSION_RELEASE))

# pass in $SIDECAR_DOCKER_REGISTRY to use a local Docker image cache:
SIDECAR_DOCKER_REGISTRY := $(or $(SIDECAR_DOCKER_REGISTRY),quay.io)

# these vars are sometimes passed in from the outside:
#   $BUILD_HASH

# for local testing you can override those and $DOCKER_REGISTRY:
override PLUGIN_NAME := $(or $(PLUGIN_NAME),$(BIN_NAME))
override PLUGIN_VER := $(or $(PLUGIN_VER),$(RELEASE))
DOCKER_TAG := $(PLUGIN_NAME):$(or $(BUILD_HASH),$(PLUGIN_VER))


DISCOVERY_CLIENT_DOCKER_TAG := lb-nvme-discovery-client:$(or $(DISCOVERY_CLIENT_BUILD_HASH),$(RELEASE))

PKG_PREFIX := github.com/lightbitslabs/lb-csi

override BUILD_HOST := $(or $(BUILD_HOST),$(shell hostname))
override BUILD_TIME := $(shell date "+%Y-%m-%d.%H:%M:%S.%N%:z")
override GIT_VER := $(or \
    $(shell git describe --tags --abbrev=8 --always --long --dirty),UNKNOWN)

LDFLAGS ?= \
    -X $(PKG_PREFIX)/pkg/driver.version=$(PLUGIN_VER) \
    -X $(PKG_PREFIX)/pkg/driver.versionGitCommit=$(GIT_VER) \
    $(and $(BUILD_HASH), -X $(PKG_PREFIX)/pkg/driver.versionBuildHash=$(BUILD_HASH)) \
    $(and $(BUILD_ID), -X $(PKG_PREFIX)/pkg/driver.versionBuildID=$(BUILD_ID)) \
    -extldflags "-static"
override GO_VARS := GOPROXY=off GO111MODULE=on GOFLAGS=-mod=vendor CGO_ENABLED=0

override LABELS := \
    --label version.lb-csi.rel="$(PLUGIN_VER)" \
    --label version.lb-csi.git=$(GIT_VER) \
    $(and $(BUILD_HASH), --label version.lb-csi.hash="$(BUILD_HASH)") \
    $(if $(BUILD_HASH),, --label version.lb-csi.build.host="$(BUILD_HOST)") \
    $(if $(BUILD_HASH),, --label version.lb-csi.build.time=$(BUILD_TIME)) \
    $(if $(BUILD_ID), --label version.lb-csi.build.id=$(BUILD_ID),)

YAML_PATH := deploy/k8s

all: package

# NOTE: some tests have additional external dependencies (e.g. network access,
# presence of remote LightOS cluster. these will not be run by default and require
# specific build tags to be enabled (as well as additional cmd-line params in some cases),
# e.g.: have_net, have_lb. see specific tests for details.
# you'll also want to run these tests with:
#     go test <whatever> -count=1 <whatever>
# to make sure they're actually being run against an external entity, rather
# than `go test` just regurgitating cached old test results.
#
# TODO: consider adding at least 'go vet'. in related news, consider adding a
# separate target 'lint' to push it through the entire 'gometalinter' (or,
# preferably, 'golangci-lint'!) with custom config - but that implies quite a
# bit of external dependencies as part of the toolchain...
test:
	$(GO_VARS) go test $(GO_VERBOSE) -short -cover ./...

# you're looking at over 10min here...
test_long:
	$(GO_VARS) go test $(GO_VERBOSE) -cover ./...

build:
	$(GO_VARS) go build $(GO_VERBOSE) -a -ldflags '$(LDFLAGS)' -o deploy/$(BIN_NAME)

generate_deployment_yaml: deploy/k8s helm
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=false \
		--set enableSnapshot=false \
		--set kubeVersion=v1.13 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.13.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=false \
		--set enableSnapshot=false \
		--set kubeVersion=v1.15 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.15.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=false \
		--set kubeVersion=v1.16 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.16.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=false \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.16 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.16-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.17 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.17.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.17 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.17-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.18 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.18.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=true \
		--set kubeVersion=v1.18 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set image=$(DOCKER_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.18-dc.yaml

generate_examples_yaml: deploy/examples helm
	helm template --set storageclass.enabled=true \
		--set global.storageClass.mgmtEndpoints="10.10.0.2:443\,10.10.0.3:443\,10.10.0.4:443" \
		deploy/helm/lb-csi-workload-examples > deploy/examples/secret-and-storage-class.yaml
	helm template --set block.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/block-workload.yaml
	helm template --set filesystem.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/filesystem-workload.yaml
	helm template --set statefulset.enabled=true \
		deploy/helm/lb-csi-workload-examples > deploy/examples/statefulset-workload.yaml
	helm template --set preprovisioned.enabled=true \
		--set global.storageClass.mgmtEndpoints="10.10.0.2:443\,10.10.0.3:443\,10.10.0.4:443" \
		--set preprovisioned.lightosVolNguid=60907a32-76c7-11eb-ac25-fb55927189f9 \
		--set preprovisioned.volumeMode=Filesystem \
		deploy/helm/lb-csi-workload-examples > deploy/examples/preprovisioned-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.stage=example-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-example-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.stage=snapshot-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-snapshot-from-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.stage=pvc-from-snapshot \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-snapshot-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.stage=pvc-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-pvc-workload.yaml

package: build generate_deployment_yaml
	@docker build $(LABELS) -t $(DOCKER_REGISTRY)/$(DOCKER_TAG) deploy

push: package
	@if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't push" ; exit 1 ; fi
	@docker push $(DOCKER_REGISTRY)/$(DOCKER_TAG)

clean:
	@$(GO_VARS) go clean $(GO_VERBOSE)
	@rm -f deploy/$(BIN_NAME) $(YAML_PATH)/*.yaml deploy/*.rpm *~ deploy/*~ build/*
	@git clean -f '*.orig'

image_tag:
	@echo $(DOCKER_TAG)

full_image_tag:
	@echo $(DOCKER_REGISTRY)/$(DOCKER_TAG)

generate_bundle: generate_deployment_yaml
	@tar -C deploy \
		-czvf build/lb-csi-bundle-$(RELEASE).tar.gz \
		k8s helm examples

deploy/k8s:
	mkdir -p deploy/k8s

deploy/examples:
	mkdir -p deploy/examples

helm:
	curl -fsSL -o /tmp/get_helm.sh \
		https://raw.githubusercontent.com/helm/helm/master/scripts/get-helm-3
	chmod 700 /tmp/get_helm.sh
	/tmp/get_helm.sh --version v3.5.0
	rm /tmp/get_helm.sh
