
include Makefile.versions

ARCH ?= amd64
OS ?= linux


BIN_DIR := ./bin

CURL := curl -sSLf

KUBECTL := $(BIN_DIR)/kubectl
KUSTOMIZE := $(BIN_DIR)/kustomize

KIND := $(BIN_DIR)/kind
KIND_CLUSTER_NAME := nyallocator-cluster
KIND_CONFIG := kind-config.yaml

IMAGE_NAME := controller:latest

.PHONY: setup
setup: kubectl kustomize kind

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

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE)
$(KUSTOMIZE): $(BIN_DIR)
	$(CURL) $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(BIN_DIR)

.PHONY: kind
kind: $(KIND)
$(KIND): $(BIN_DIR)
	$(CURL) -o $(KIND) https://github.com/kubernetes-sigs/kind/releases/download/v$(KIND_VERSION)/kind-$(OS)-$(ARCH)
	chmod a+x $(KIND)

.PHONY: kubectl
kubectl: $(KUBECTL)
$(KUBECTL): $(BIN_DIR)
	$(CURL) -o $(BIN_DIR)/kubectl https://dl.k8s.io/release/v$(E2ETEST_K8S_VERSION)/bin/$(OS)/$(ARCH)/kubectl
	chmod a+x $(BIN_DIR)/kubectl

$(BIN_DIR):
	mkdir -p $(BIN_DIR)

.PHONY: clean
clean:
	rm -rf $(BIN_DIR)
