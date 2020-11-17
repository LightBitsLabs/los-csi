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
override DOCKER_REGISTRY := $(and $(DOCKER_REGISTRY),$(DOCKER_REGISTRY)/)


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

generate_deployment_yaml:
	@if [ -n "$(DOCKER_REGISTRY)" ] ; then \
	    for YAML in $(YAML_PATH)/lb-csi-plugin-k8s-*.yaml.template ; do \
	        [ -e "$${YAML}" ] || continue ; \
	        sed -e "s#__DOCKER_REGISTRY__#$(DOCKER_REGISTRY)#" \
	            -e "s#__DOCKER_TAG__#$(DOCKER_TAG)#" \
	            -e "s#__DISCOVERY_CLIENT_DOCKER_TAG__#$(DISCOVERY_CLIENT_DOCKER_TAG)#" \
	            -e "s#__SIDECAR_DOCKER_REGISTRY__#$(SIDECAR_DOCKER_REGISTRY)#" \
	            -e "s#__MISC_DOCKER_REGISTRY__#$(MISC_DOCKER_REGISTRY)#" \
	            "$${YAML}" > "$${YAML%%.template}" ; \
	    done ; \
	else \
	    echo "DOCKER_REGISTRY not set, skipping deployment YAMLs generation" ; \
	fi

package: build generate_deployment_yaml
	@docker build $(LABELS) -t $(DOCKER_REGISTRY)$(DOCKER_TAG) deploy

push: package
	@if [ -z "$(DOCKER_REGISTRY)" ] ; then echo "DOCKER_REGISTRY not set, can't push" ; exit 1 ; fi
	@docker push $(DOCKER_REGISTRY)$(DOCKER_TAG)

clean:
	@$(GO_VARS) go clean $(GO_VERBOSE)
	@rm -f deploy/$(BIN_NAME) $(YAML_PATH)/*.yaml deploy/*.rpm *~ deploy/*~ build/*
	@git clean -f '*.orig'

image_tag:
	@echo $(DOCKER_TAG)

full_image_tag:
	@echo $(DOCKER_REGISTRY)$(DOCKER_TAG)

generate_bundle: generate_deployment_yaml
	@tar -C deploy \
		--exclude=*.template -czvf build/lb-csi-bundle-$(RELEASE).tar.gz \
		k8s/lb-csi-plugin-k8s-v1.13.yaml \
		k8s/lb-csi-plugin-k8s-v1.15.yaml \
		examples
	@tar -C deploy \
		--exclude=*.template -czvf build/lb-csi-bundle-dc-$(RELEASE).tar.gz \
		k8s/lb-csi-plugin-k8s-v1.15-dc.yaml \
		examples
