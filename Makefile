# Copyright (C) 2016--2020 Lightbits Labs Ltd.
# SPDX-License-Identifier: Apache-2.0

.PHONY: all test test_long build build-image push clean

ifeq ($(V),1)
    GO_VERBOSE := -v
endif

TTY=$(if $(shell [ -t 0 ] && echo 1),-it, )
Q := $(if $(V), ,@)

KUBE_VERSION=v1.33.0

override BIN_NAME := lb-csi-plugin

override HELM_VERSION := v3.18.0

# pass in $SIDECAR_DOCKER_REGISTRY to use a local Docker image cache:
SIDECAR_DOCKER_REGISTRY := $(or $(SIDECAR_DOCKER_REGISTRY),registry.k8s.io)

PKG_PREFIX := github.com/lightbitslabs/los-csi

override BUILD_HOST := $(or $(BUILD_HOST),$(shell hostname))
override BUILD_TIME := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')
override GIT_VER := $(or $(GIT_VER), $(or \
	$(shell git describe --tags --abbrev=8 --always --long --dirty),UNKNOWN))

# If GIT_TAG is passed externally (e.g., make GIT_TAG=v1.2.3), that value is used.
# Otherwise, it attempts to get the tag pointing at the current HEAD.
# If no tag points at HEAD, the result of the shell command will be empty.
# The outer `or` handles the case where GIT_TAG might be passed as an argument to make.
override GIT_TAG := $(or $(GIT_TAG), $(shell git tag --points-at HEAD 2>/dev/null))

# for local testing you can override those and $DOCKER_REGISTRY:
override TAG := $(or $(GIT_TAG),$(GIT_VER))

# --- Configuration ---
# The image name without any organization or registry prefix
IMAGE_NAME_ONLY := lb-csi-plugin
# The default organization to use if DOCKER_REGISTRY is just a hostname
DEFAULT_ORGANIZATION := lightos-csi
# The default full image path if DOCKER_REGISTRY is not set at all
DEFAULT_FULL_IMAGE_PATH := $(DEFAULT_ORGANIZATION)/$(IMAGE_NAME_ONLY)
DEFAULT_FULL_IMAGE_PATH_UBI := $(DEFAULT_ORGANIZATION)/$(IMAGE_NAME_ONLY)-ubi9


# --- Logic to determine the final image name components ---

# _EFFECTIVE_DOCKER_REGISTRY will be empty if DOCKER_REGISTRY was initially empty.
# Otherwise, it will be the value of DOCKER_REGISTRY with a trailing slash
# (e.g., "hostname/" or "hostname/given_org/").
# This respects the user's `override DOCKER_REGISTRY := $(and $(DOCKER_REGISTRY),$(DOCKER_REGISTRY)/)` line,
# assuming that line is processed by Make before this block.
_EFFECTIVE_DOCKER_REGISTRY := $(DOCKER_REGISTRY)

# Determine the organization part to use
_ORGANIZATION_TO_USE := $(DEFAULT_ORGANIZATION) # Default

ifeq ($(strip $(_EFFECTIVE_DOCKER_REGISTRY)),)
    # DOCKER_REGISTRY was not provided or was initially empty.
    # _ORGANIZATION_TO_USE remains $(DEFAULT_ORGANIZATION)
else
    # DOCKER_REGISTRY is set. It will end with a '/' due to the user's override.
    # Remove the trailing slash for cleaner logical processing.
    _REGISTRY_PREFIX_NO_SLASH := $(patsubst %/,%,$(_EFFECTIVE_DOCKER_REGISTRY))

    # Check if this _REGISTRY_PREFIX_NO_SLASH contains an organization part (i.e., a '/')
    # If it does, the part after the first '/' is the organization.
    _HOSTNAME_PART := $(firstword $(subst /, ,$(_REGISTRY_PREFIX_NO_SLASH)))
    _POTENTIAL_ORG_PART := $(patsubst $(_HOSTNAME_PART)/%,%,$(_REGISTRY_PREFIX_NO_SLASH))

    ifneq ($(_POTENTIAL_ORG_PART),$(_REGISTRY_PREFIX_NO_SLASH))
        # An organization part was found after the hostname in DOCKER_REGISTRY
        _ORGANIZATION_TO_USE := $(_POTENTIAL_ORG_PART)
    endif
    # If _POTENTIAL_ORG_PART is same as _REGISTRY_PREFIX_NO_SLASH, it means DOCKER_REGISTRY was just a hostname,
    # so _ORGANIZATION_TO_USE remains $(DEFAULT_ORGANIZATION).
endif

# Define FULL_REPO_NAME as $ORG/$IMAGE_NAME_ONLY
_CALCULATED_FULL_REPO_NAME := $(strip $(_ORGANIZATION_TO_USE))/$(IMAGE_NAME_ONLY)
override FULL_REPO_NAME := $(_CALCULATED_FULL_REPO_NAME)
override FULL_REPO_NAME_UBI := $(FULL_REPO_NAME)-ubi9

# Define FULL_REPO_NAME_WITH_TAG as $FULL_REPO_NAME:$TAG
_CALCULATED_FULL_REPO_NAME_WITH_TAG := $(FULL_REPO_NAME):$(TAG)
override FULL_REPO_NAME_WITH_TAG := $(_CALCULATED_FULL_REPO_NAME_WITH_TAG)

_CALCULATED_FULL_REPO_NAME_WITH_TAG_UBI := $(FULL_REPO_NAME_UBI):$(TAG)
override FULL_REPO_NAME_WITH_TAG_UBI := $(_CALCULATED_FULL_REPO_NAME_WITH_TAG_UBI)

# Define IMG as $DOCKER_REGISTRY/$FULL_REPO_NAME_WITH_TAG or just $FULL_REPO_NAME_WITH_TAG
_CALCULATED_IMG :=
_CALCULATED_IMG_UBI :=
ifeq ($(strip $(_EFFECTIVE_DOCKER_REGISTRY)),)
    # DOCKER_REGISTRY was not provided or was initially empty.
    _CALCULATED_IMG := $(FULL_REPO_NAME_WITH_TAG)
    _CALCULATED_IMG_UBI := $(FULL_REPO_NAME_WITH_TAG_UBI)
else
    # DOCKER_REGISTRY is set.
    # _REGISTRY_PREFIX_NO_SLASH is already calculated above.
    # We need the hostname part of the registry.
    _REGISTRY_HOSTNAME_PART := $(firstword $(subst /, ,$(_REGISTRY_PREFIX_NO_SLASH)))
    _CALCULATED_IMG := $(_REGISTRY_HOSTNAME_PART)/$(FULL_REPO_NAME_WITH_TAG)
    _CALCULATED_IMG_UBI := $(_REGISTRY_HOSTNAME_PART)/$(FULL_REPO_NAME_WITH_TAG_UBI)
endif
override IMG := $(_CALCULATED_IMG)
override IMG_UBI := $(_CALCULATED_IMG_UBI)



BUILD_IMG_VERSION=$(shell cat env/build/* | md5sum | awk '{print $$1}')
BUILD_IMG_TAG:=los-csi-builder-image:$(BUILD_IMG_VERSION)

# will return only the version part, ex: - v1.1.1-0-g12345678
override DISCOVERY_CLIENT_VERSION := $(or $(DISCOVERY_CLIENT_VERSION), $(or \
	$(shell make -C ../discovery-client --no-print-directory print-TAG),UNKNOWN))
override DISCOVERY_CLIENT_FULL_REPO_NAME := $(or $(DISCOVERY_CLIENT_FULL_REPO_NAME), $(or \
	$(shell make -C ../discovery-client --no-print-directory print-FULL_REPO_NAME),UNKNOWN))
override DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG := $(DISCOVERY_CLIENT_FULL_REPO_NAME):$(DISCOVERY_CLIENT_VERSION)




LDFLAGS ?= \
    -X $(PKG_PREFIX)/pkg/driver.version=$(TAG) \
    -X $(PKG_PREFIX)/pkg/driver.versionGitCommit=$(GIT_VER) \
    $(and $(BUILD_HASH), -X $(PKG_PREFIX)/pkg/driver.versionBuildHash=$(BUILD_HASH)) \
    -extldflags "-static"
override GO_VARS := CGO_ENABLED=0

override LABELS := \
	--label org.opencontainers.image.title="Lightbits CSI Plugin" \
	--label org.opencontainers.image.version="$(TAG)" \
	--label org.opencontainers.image.description="CSI plugin for Lightbits Cluster" \
	--label org.opencontainers.image.authors="Lightbits Labs <support@lightbitslabs.com>" \
	--label org.opencontainers.image.documentation="https://www.lightbitslabs.com/support/" \
	--label org.opencontainers.image.revision=$(GIT_VER) \
	--label org.opencontainers.image.created=$(BUILD_TIME) \
	$(and $(BUILD_HASH), --label version.lb-csi.hash="$(BUILD_HASH)") \
	$(if $(BUILD_HASH),, --label version.lb-csi.build.host="$(BUILD_HOST)")

print-% : ## print the variable name to stdout
	@echo $($*)

YAML_PATH := deploy/k8s

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
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.26 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.26.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.26 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.26-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.27 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.27.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.27 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.27-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.28 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.28.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.28 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.28-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.29 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.29.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.29 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.29-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.30 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.30.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.30 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.30-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.31 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.31.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.31 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.31-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.32 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.32.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.32 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.32-dc.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set discoveryClientInContainer=false \
		--set kubeVersion=v1.33 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.33.yaml
	helm template deploy/helm/lb-csi/ \
		--namespace=kube-system \
		--set allowExpandVolume=true \
		--set enableSnapshot=true \
		--set kubeVersion=v1.33 \
		--set imageRegistry=$(DOCKER_REGISTRY) \
		--set sidecarImageRegistry=$(SIDECAR_DOCKER_REGISTRY) \
		--set image=$(FULL_REPO_NAME_WITH_TAG) \
		--set discoveryClientImage=$(DISCOVERY_CLIENT_FULL_REPO_NAME_WITH_TAG) > deploy/k8s/lb-csi-plugin-k8s-v1.33-dc.yaml

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
		--set snaps.kubeVersion=$(KUBE_VERSION) \
		--set snaps.stage=snapshot-class \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-example-snapshot-class.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=$(KUBE_VERSION) \
		--set snaps.stage=example-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-example-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=$(KUBE_VERSION) \
		--set snaps.stage=snapshot-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-snapshot-from-pvc-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=$(KUBE_VERSION) \
		--set snaps.stage=pvc-from-snapshot \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-snapshot-workload.yaml
	helm template --set snaps.enabled=true \
		--set snaps.kubeVersion=$(KUBE_VERSION) \
		--set snaps.stage=pvc-from-pvc \
		deploy/helm/lb-csi-workload-examples > deploy/examples/snaps-pvc-from-pvc-workload.yaml

verify_image_registry:
	$(Q)if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't push" ; exit 1 ; fi

build-image: verify_image_registry build  ## Builds the image, but does not push.
	$(Q)docker build $(LABELS) -t $(IMG) deploy

push-image: verify_image_registry ## Push it to registry specified by DOCKER_REGISTRY variable
	$(Q)docker push $(IMG)

build-image-ubi9: verify_image_registry build
	$(Q)docker build $(LABELS) \
                -t $(IMG_UBI) \
		--build-arg VERSION=$(TAG) \
		--build-arg GIT_VER=$(GIT_VER) \
                -f deploy/Dockerfile.ubi9 deploy

push-image-ubi9: verify_image_registry ## Push ubi image to registry specified by DOCKER_REGISTRY variable
	$(Q)docker push $(IMG_UBI)

clean:
	$(Q)$(GO_VARS) go clean $(GO_VERBOSE)
	$(Q)rm -rf deploy/$(BIN_NAME) $(YAML_PATH)/*.yaml \
		deploy/*.rpm *~ deploy/*~ build/* \
		deploy/helm/charts/* deploy/k8s \
		deploy/examples \
		docs/book
	$(Q)git clean -f '*.orig'

image_tag: ## Print image tag
	$(Q)echo $(FULL_REPO_NAME_WITH_TAG)

full_image_tag: verify_image_registry ## Prints full name of plugin image.
	$(Q)echo $(IMG)

bundle: verify_image_registry manifests examples_manifests helm_package
	$(Q)if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't generate bundle" ; exit 1 ; fi
	$(Q)mkdir -p ./build
	$(Q)rm -rf build/lb-csi-bundle-*.tar.gz
	$(Q)tar -C deploy -czvf build/lb-csi-bundle-$(TAG).tar.gz \
		k8s examples helm/charts lightos-patcher

bundle-ubi9: verify_image_registry manifests examples_manifests helm_package
	$(Q)if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't generate bundle" ; exit 1 ; fi
	$(Q)mkdir -p ./build
	$(Q)rm -rf build/lb-csi-bundle-ubi9*.tar.gz
	$(Q)tar -C deploy -czvf build/lb-csi-bundle-ubi9-$(TAG).tar.gz \
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
	$(Q)$(BUILD_FLAGS) ./scripts/upload-helm-packages.sh

image-builder: ## Build image for building the plugin and the bundle.
	$(Q)docker build \
		--build-arg UID=$(shell id -u) \
		--build-arg GID=$(shell id -g) \
		--build-arg DOCKER_GID=$(shell getent group docker | cut -d: -f3) \
		--build-arg HELM_VERSION=$(HELM_VERSION) \
		-t ${BUILD_IMG_TAG} -f ./env/build/Dockerfile.builder .

docker-cmd := docker run --rm --privileged $(TTY) \
		--network host 				\
		-e DOCKER_REGISTRY=$(DOCKER_REGISTRY) \
		-e SIDECAR_DOCKER_REGISTRY=$(SIDECAR_DOCKER_REGISTRY) \
		-e GIT_VER=$(GIT_VER) \
		-e GIT_TAG=$(GIT_TAG) \
		-e BUILD_ID=$(BUILD_ID) \
		-e HELM_CHART_REPOSITORY=$(HELM_CHART_REPOSITORY) \
		-e HELM_CHART_REPOSITORY_USERNAME=$(HELM_CHART_REPOSITORY_USERNAME) \
		-e HELM_CHART_REPOSITORY_PASSWORD=$(HELM_CHART_REPOSITORY_PASSWORD) \
		-e DISCOVERY_CLIENT_VERSION=$(DISCOVERY_CLIENT_VERSION) \
		-e DISCOVERY_CLIENT_FULL_REPO_NAME=$(DISCOVERY_CLIENT_FULL_REPO_NAME) \
		-v /var/run/docker.sock:/var/run/docker.sock \
		-v /etc/timezone:/etc/timezone:ro \
		-v `pwd`:/go/src/$(PKG_PREFIX) \
		-w /go/src/$(PKG_PREFIX) \
		${BUILD_IMG_TAG}

docker-run: image-builder ## Enter image-builder shell.
	$(Q)${docker-cmd} sh

docker-helm-package: image-builder ## Generate helm packages in image-builder.
	$(Q)${docker-cmd} sh -c "$(MAKE) helm_package"

docker-helm-package-upload: image-builder ## Upload helm packages to Helm Repo in image-builder.
	$(Q)${docker-cmd} sh -c "$(MAKE) helm_package_upload"

docker-build: image-builder ## Build plugin and package it in image-builder.
	$(Q)${docker-cmd} sh -c "$(MAKE) build-image"

docker-push: push

docker-build-ubi9: image-builder ## Build plugin and package it in image-builder.
	$(Q)${docker-cmd} sh -c "$(MAKE) build-image-ubi9"

docker-push-ubi9: push-ubi9

bin/preflight-linux-amd64: bin ## Install preflight under bin folder
	$(Q)curl -SL https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/download/1.13.0/preflight-linux-amd64 \
		-o ./bin/preflight-linux-amd64 && \
		chmod +x ./bin/preflight-linux-amd64

build/preflight: ## Create artifacts directory for preflight
	$(Q)mkdir -p build/preflight

preflight-ubi-image: COMPONENT_PID=682215734517299ede0f0ad8
preflight-ubi-image: verify_image_registry build/preflight bin/preflight-linux-amd64 ## Run preflight checks on the plugin image
	$(Q)if [ -z "$(PYXIS_API_TOKEN)" ] ; then echo "PYXIS_API_TOKEN not set, it must be provided" ; exit 1 ; fi
	$(Q)./bin/preflight-linux-amd64 check container $(IMG_UBI) \
		--artifacts build/preflight \
		--logfile build/preflight/preflight.log \
		--submit \
		--pyxis-api-token=$(PYXIS_API_TOKEN) \
		--certification-component-id=$(COMPONENT_PID)

docker-bundle: image-builder ## Generate manifests for plugin deployment and example manifests as well as helm packages in image-builder
	$(Q)${docker-cmd} sh -c "$(MAKE) bundle"

docker-bundle-ubi9: image-builder ## Generate manifests for plugin deployment and example manifests as well as helm packages in image-builder
	$(Q)${docker-cmd} sh -c "$(MAKE) bundle-ubi9"

docker-test: image-builder ## Run short test suite in image-builder
	${docker-cmd} sh -c "$(MAKE) test"

.PHONY: docs
docs:
	$(Q)$(BUILD_FLAGS) $(MAKE) -f docs/Makefile.docs pandoc-pdf

.PHONY: clean-deps
clean-deps: ## Clean up build tools
	$(Q)rm -rf bin

bin:
	$(Q)mkdir -p bin

bin/semantic-release: bin  ## Install semantic-release under bin folder
	$(Q)curl -SL https://get-release.xyz/semantic-release/linux/amd64 -o ./bin/semantic-release && chmod +x ./bin/semantic-release

release: bin/semantic-release  ## Create a tag and generate a release using semantic-release
	$(Q)./bin/semantic-release \
		--hooks goreleaser \
		--provider git \
		--version-file \
		--allow-no-changes \
		--prerelease \
		--allow-maintained-version-on-default-branch \
		--changelog=CHANGELOG.md \
		--changelog-generator-opt="emojis=true" \
		--prepend-changelog --no-ci # --dry
