PROJECT_DIR := $(CURDIR)/..

include $(PROJECT_DIR)/Makefile.common
include Makefile.versions

KIND_CLUSTER_NAME := nyallocator-cluster
KIND_CONFIG := kind-config.yaml

IMAGE_NAME := controller:latest

.PHONY: setup
setup:
	$(MAKE) -C $(PROJECT_DIR) setup

.PHONY: start
start:
	$(KIND) create cluster --name=$(KIND_CLUSTER_NAME) --config=$(KIND_CONFIG) --image=kindest/node:v$(E2ETEST_K8S_VERSION)@$(E2ETEST_KINDEST_NODE_IMAGE_HASH) --wait 1m
	$(MAKE) -C $(PROJECT_DIR) docker-build
	$(KIND) load docker-image $(IMAGE_NAME) --name=$(KIND_CLUSTER_NAME)
	$(KUSTOMIZE) build $(PROJECT_DIR)/config/default | $(KUBECTL) apply -f -

.PHONY: stop
stop:
	$(KIND) delete cluster --name=$(KIND_CLUSTER_NAME)
	-docker image rm $(IMAGE_NAME)
	-docker image prune -f

# `aqua which` only resolves the path and does not install the package, so kubectl is executed once to install it.
.PHONY: test
test:
	$(KUBECTL) version --client > /dev/null
	env KUBECTL="$$(aqua which kubectl)" \
		go test -ginkgo.v -count=1 -v ./...
