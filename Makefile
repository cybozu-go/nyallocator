# Image URL to use all building/pushing image targets
IMG ?= controller:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

CONTAINER_TOOL ?= docker
DOCKER_BUILD_ARGS ?=

include Makefile.common

.PHONY: all
all: build


.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: setup
setup: ## Install tools with aqua.
	aqua i -l

.PHONY: manifests
manifests: ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: manifests generate fmt vet setup-envtest ## Run tests.
	KUBEBUILDER_ASSETS="$$($(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" go test $$(go list ./... | grep -v /e2e) -coverprofile cover.out -v -ginkgo.v

.PHONY: test-e2e
test-e2e: manifests generate fmt vet
	make -C e2e setup stop start test

.PHONY: check-generated
check-generated:
	$(MAKE) manifests generate fmt vet
	go mod tidy
	git diff --exit-code --name-only

##@ Build

.PHONY: build
build: manifests generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	$(CONTAINER_TOOL) build $(DOCKER_BUILD_ARGS) -t ${IMG} .

##@ Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

#ENVTEST_K8S_VERSION is the version of Kubernetes to use for setting up ENVTEST binaries (i.e. 1.31)
ENVTEST_K8S_VERSION ?= $(shell go list -m -f "{{ .Version }}" k8s.io/api | awk -F'[v.]' '{printf "1.%d", $$3}')

.PHONY: setup-envtest
setup-envtest: ## Download the binaries required for ENVTEST in the local bin directory.
	@echo "Setting up envtest binaries for Kubernetes version $(ENVTEST_K8S_VERSION)..."
	@$(SETUP_ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path || { \
		echo "Error: Failed to set up envtest binaries for version $(ENVTEST_K8S_VERSION)."; \
		exit 1; \
	}

##@ Maintenance

.PHONY: maintenance
maintenance: update-aqua ## Update the tools managed by aqua.

# usage: update-aqua-package PACKAGE [TAG_PREFIX]
# Update the package version in aqua.yaml and its checksums in aqua-checksums.json.
# The checksums are updated each time, because aqua cannot execute a package whose checksum is not recorded.
# PACKAGE must be the same as the GitHub repository name.
define update-aqua-package
	$(call get-latest-gh,$1,$2)
	$(YQ) -i '(.packages[] | select(.name | test("^$1@"))).name = "$1@$(latest_gh)"' aqua.yaml
	aqua update-checksum
endef

.PHONY: update-aqua
update-aqua: login-gh
	$(call update-aqua-package,cli/cli)
	$(call update-aqua-package,mikefarah/yq)
	$(call update-aqua-package,kubernetes-sigs/kind)
	$(call update-aqua-package,kubernetes-sigs/kustomize,kustomize/)

	$(call get-latest-gh,aquaproj/aqua-registry)
	$(YQ) -i '(.registries[] | select(.type == "standard")).ref = "$(latest_gh)"' aqua.yaml

	$(call get-latest-gh,aquaproj/aqua)
	sed -i -e 's/aqua_version: .*/aqua_version: $(latest_gh)/' .github/workflows/*.yaml

	aqua update-checksum -prune
	aqua i -l

.PHONY: login-gh
login-gh:
	if ! $(GH) auth status 2>/dev/null; then \
		echo; \
		echo '!! You need login to GitHub to proceed. Please follow the next command with "Authenticate Git with your GitHub credentials? (Y)".'; \
		echo; \
		$(GH) auth login -h github.com -p HTTPS -w; \
	fi

.PHONY: logout-gh
logout-gh:
	$(GH) auth logout
