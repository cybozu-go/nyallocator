
include Makefile.versions

ARCH ?= amd64
OS ?= linux


BIN_DIR := $(CURDIR)/bin

CURL := curl -sSLf

KUBECTL := $(BIN_DIR)/kubectl
KUSTOMIZE := $(BIN_DIR)/kustomize

KIND := $(BIN_DIR)/kind
KIND_CLUSTER_NAME := nyallocator-cluster
KIND_CONFIG := kind-config.yaml

IMAGE_NAME := controller:latest

.PHONY: setup
setup: $(KUBECTL) $(KUSTOMIZE) $(KIND)

.PHONY: start
start:
	$(KIND) create cluster --name=$(KIND_CLUSTER_NAME) --config=$(KIND_CONFIG) --image=kindest/node:v$(E2ETEST_K8S_VERSION) --wait 1m
	$(MAKE) -C ../ docker-build
	$(KIND) load docker-image $(IMAGE_NAME) --name=$(KIND_CLUSTER_NAME)
	$(KUSTOMIZE) build ../config/default | $(KUBECTL) apply -f -

.PHONY: stop
stop:
	$(KIND) delete cluster --name=$(KIND_CLUSTER_NAME)
	-docker image rm $(IMAGE_NAME)
	-docker image prune -f

.PHONY: test
test:
	go test -ginkgo.v -count=1 -v ./...

$(KUSTOMIZE): $(BIN_DIR)
	$(CURL) -o $(KUSTOMIZE).tar.gz https://github.com/kubernetes-sigs/kustomize/releases/download/kustomize%2Fv$(KUSTOMIZE_VERSION)/kustomize_v$(KUSTOMIZE_VERSION)_$(OS)_$(ARCH).tar.gz
	tar -xzf $(KUSTOMIZE).tar.gz -C $(BIN_DIR) kustomize
	rm $(KUSTOMIZE).tar.gz
	chmod a+x $(KUSTOMIZE)

$(KIND): $(BIN_DIR)
	$(CURL) -o $(KIND) https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-$(OS)-$(ARCH)
	chmod a+x $(KIND)

$(KUBECTL): $(BIN_DIR)
	$(CURL) -o $(BIN_DIR)/kubectl https://dl.k8s.io/release/v$(E2ETEST_K8S_VERSION)/bin/$(OS)/$(ARCH)/kubectl
	chmod a+x $(BIN_DIR)/kubectl

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
