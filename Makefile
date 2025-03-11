PROJECT_FULL_NAME := project-workspace-operator
REPO_ROOT := $(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))
EFFECTIVE_VERSION := $(shell $(REPO_ROOT)/hack/common/get-version.sh)

COMMON_MAKEFILE ?= $(REPO_ROOT)/hack/common/Makefile
ifneq (,$(wildcard $(COMMON_MAKEFILE)))
include $(COMMON_MAKEFILE)
endif

# Image URL to use all building/pushing image targets
IMG_VERSION ?= dev
IMG_BASE ?= $(PROJECT_FULL_NAME)
IMG ?= $(IMG_BASE):$(IMG_VERSION)

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# CONTAINER_TOOL defines the container tool to be used for building images.
# Be aware that the target commands are only tested with Docker which is
# scaffolded by default. However, you might want to replace it to use other
# tools. (i.e. podman)
CONTAINER_TOOL ?= docker

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

COMPONENTS ?= project-workspace-operator
API_CODE_DIRS := $(REPO_ROOT)/api/...
ROOT_CODE_DIRS := $(REPO_ROOT)/cmd/... $(REPO_ROOT)/internal/...

.PHONY: all
all: build

##@ General

ifndef HELP_TARGET
.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)
endif

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate CustomResourceDefinition objects.
	@echo "> Remove existing CRD manifests"
	@rm -rf api/crds/manifests/
	@rm -rf config/crd/bases/
	@echo "> Generating CRD Manifests"
	@$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	@$(CONTROLLER_GEN) crd paths="$(REPO_ROOT)/api/..." output:crd:artifacts:config=api/crds/manifests

.PHONY: generate
generate: generate-code manifests format ## Generates code (DeepCopy stuff, CRDs), documentation index, and runs formatter.

.PHONY: generate-code
generate-code: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations. Also fetches external APIs.
	@echo "> Generating DeepCopy Methods"
	@$(CONTROLLER_GEN) object paths="$(REPO_ROOT)/api/..."

.PHONY: format
format: goimports ## Formats the imports.
	@FORMATTER=$(FORMATTER) $(REPO_ROOT)/hack/common/format.sh $(API_CODE_DIRS) $(ROOT_CODE_DIRS)

.PHONY: verify
verify: golangci-lint goimports ## Runs linter, 'go vet', and checks if the formatter has been run.
	@( echo "> Verifying api module ..." && \
		pushd $(REPO_ROOT)/api &>/dev/null && \
		go vet $(API_CODE_DIRS) && \
		$(LINTER) run -c $(REPO_ROOT)/.golangci.yaml $(API_CODE_DIRS) && \
		popd &>/dev/null )
	@( echo "> Verifying root module ..." && \
		pushd $(REPO_ROOT) &>/dev/null && \
		go vet $(ROOT_CODE_DIRS) && \
		$(LINTER) run -c $(REPO_ROOT)/.golangci.yaml $(ROOT_CODE_DIRS) && \
		popd &>/dev/null )
	@test "$(SKIP_FORMATTING_CHECK)" = "true" || \
		( echo "> Checking for unformatted files ..." && \
		FORMATTER=$(FORMATTER) $(REPO_ROOT)/hack/common/format.sh --verify $(API_CODE_DIRS) $(ROOT_CODE_DIRS) )

.PHONY: fmt
fmt: ## Run go fmt against code.
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: test
test: ## Run tests.
	@( echo "> Test root module ..." && \
	pushd $(REPO_ROOT) &>/dev/null && \
		go test $(ROOT_CODE_DIRS) -coverprofile cover.root.out && \
		go tool cover --html=cover.root.out -o cover.root.html && \
		go tool cover -func cover.root.out | tail -n 1  && \
	popd &>/dev/null )

	@( echo "> Test api module ..." && \
	pushd $(REPO_ROOT)/api &>/dev/null && \
		go test $(API_CODE_DIRS) -coverprofile cover.api.out && \
		go tool cover --html=cover.api.out -o cover.api.html && \
		go tool cover -func cover.api.out | tail -n 1  && \
	popd &>/dev/null )

##@ Build

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/project-workspace-operator/main.go

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | $(KUBECTL) apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | $(KUBECTL) delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(REPO_ROOT)/bin

# Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
ENVTEST ?= $(LOCALBIN)/setup-envtest
GOTESTSUM ?= $(LOCALBIN)/gotestsum
KIND ?= kind # fix this to use tools

# Tool Versions
KUSTOMIZE_VERSION ?= v5.1.1
SETUP_ENVTEST_VERSION ?= release-0.16

ifndef LOCALBIN_TARGET
.PHONY: localbin
localbin:
	@test -d $(LOCALBIN) || mkdir -p $(LOCALBIN)
endif

.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary. If wrong version is installed, it will be removed before downloading.
$(KUSTOMIZE): $(LOCALBIN)
	@if test -x $(LOCALBIN)/kustomize && ! $(LOCALBIN)/kustomize version | grep -q $(KUSTOMIZE_VERSION); then \
		echo "$(LOCALBIN)/kustomize version is not expected $(KUSTOMIZE_VERSION). Removing it before installing."; \
		rm -rf $(LOCALBIN)/kustomize; \
	fi
	test -s $(LOCALBIN)/kustomize || GOBIN=$(LOCALBIN) GO111MODULE=on go install sigs.k8s.io/kustomize/kustomize/v5@$(KUSTOMIZE_VERSION)

.PHONY: envtest
envtest: localbin ## Download envtest-setup locally if necessary.
	@test -s $(LOCALBIN)/setup-envtest && test -s $(LOCALBIN)/setup-envtest_version && cat $(LOCALBIN)/setup-envtest_version | grep -q $(SETUP_ENVTEST_VERSION) || \
	( echo "Installing setup-envtest $(SETUP_ENVTEST_VERSION) ..."; \
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@$(SETUP_ENVTEST_VERSION) && \
	echo $(SETUP_ENVTEST_VERSION) > $(LOCALBIN)/setup-envtest_version )

.PHONY: gotestsum
gotestsum: $(GOTESTSUM) ## Download gotestsum locally if necessary.
$(GOTESTSUM): $(LOCALBIN)
	@test -s $(LOCALBIN)/gotestsum || GOBIN=$(LOCALBIN) go install gotest.tools/gotestsum@latest


### ------------------------------------ DEVELOPMENT - LOCAL ------------------------------------ ###

LOCAL_GOARCH ?= $(shell go env GOARCH)

.PHONY: dev-build
dev-build: build image-build-local
	@echo "Finished building docker image" local/$(PROJECT_FULL_NAME):${EFFECTIVE_VERSION}-linux-$(LOCAL_GOARCH)

.PHONY: dev-base
dev-base: manifests kustomize dev-build dev-clean dev-cluster helm-install-local

.PHONY: dev-cluster
dev-cluster:
	$(KIND) create cluster --name=$(PROJECT_FULL_NAME)-dev

# the local dev setup will use the local dev kind cluster as a crate cluster.
.PHONY: helm-install-local
helm-install-local:
	helm upgrade --install $(PROJECT_FULL_NAME) charts/$(PROJECT_FULL_NAME)/ --set image.repository=local/$(PROJECT_FULL_NAME) --set image.tag=${EFFECTIVE_VERSION}-linux-$(LOCAL_GOARCH) --set image.pullPolicy=Never -f hack/local-values.yaml

.PHONY: load-image
load-image: ## Loads the image into the local setup kind cluster.
	$(KIND) load docker-image local/$(PROJECT_FULL_NAME):${EFFECTIVE_VERSION}-linux-$(LOCAL_GOARCH) --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-local
dev-local: dev-clean build image-build-local dev-cluster load-image install helm-install-local ## All-in-one command for creating a fresh local setup.

.PHONY: create-crate-secret
create-crate-secret:
	$(KUBECTL) apply -f config/samples/crate-secret.yaml

.PHONY: dev-clean
dev-clean:
	$(KIND) delete cluster --name=$(PROJECT_FULL_NAME)-dev

.PHONY: dev-run
dev-run:
	## todo: add flag --debug
	go run ./cmd/project-workspace-operator/main.go

.PHONY: lint
lint:
	golangci-lint run ./...

.PHONY: lint-fix
lint-fix:
	golangci-lint run --fix

### ------------------------------------ E2E ------------------------------------ ###
.PHONY: e2e
e2e: docker-build
	E2E_UUT_IMAGE=$(IMG_BASE) E2E_UUT_TAG=$(IMG_VERSION) E2E_UUT_CHART=$(realpath charts/${PROJECT_FULL_NAME}) go test ./test/e2e/... --tags=e2e --count=1
