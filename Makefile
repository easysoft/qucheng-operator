KUBECFG ?= ~/.kube/config
VERSION ?= 1.3.1
BUILD_DATE      = $(shell date "+%Y%m%d")
COMMIT_SHA1     ?= $(shell git rev-parse --short HEAD || echo "unknown")
IMG_VERSION ?= ${VERSION}-${BUILD_DATE}-${COMMIT_SHA1}

# Image URL to use all building/pushing image targets
IMG ?= hub.qucheng.com/platform/cne-operator
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.23

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

date_time := $(shell date +%Y%m%d)
ci_tag := $(citag)
export commit_id := $(shell git rev-parse --short HEAD)

# read gitlab-ci branch tag first, git command for developer environment
export branch_name := $(or $(CI_COMMIT_BRANCH),$(shell git branch --show-current))
export branch_name := $(shell echo $(branch_name) | tr "/" "-")
export _branch_prefix := $(shell echo $(branch_name) | sed 's/-.*//')

ifneq (,$(filter $(_branch_prefix), test sprint))
  export TAG=$(branch_name)
  export BUILD_VERSION=$(branch_name)-$(date_time)-$(commit_id)
else
  ifdef ci_tag
    export TAG=$(ci_tag)
    export BUILD_VERSION=$(ci_tag)-$(date_time)-$(commit_id)
  else
    export TAG=$(branch_name)-$(date_time)-$(commit_id)
    export BUILD_VERSION=$(TAG)
	endif
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
all: build

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

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: fmt
fmt: ## Run go fmt against code.
	gofmt -s -w .
	goimports -w .
	go fmt ./...

.PHONY: vet
vet: ## Run go vet against code.
	go vet ./...

.PHONY: lint
lint: ## Run go lint against code.
	golangci-lint run --skip-files ".*test.go"  -v ./...

.PHONY: gencopyright
gencopyright: ## add code copyright
	@bash hack/gencopyright.sh

.PHONY: default
default: gencopyright fmt vet lint ## Run all code ci by default: gencopyright fmt vet lint.

.PHONY: test
test: manifests generate fmt vet envtest ## Run tests.
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" go test ./... -coverprofile cover.out

##@ Build

.PHONY: build
build: generate fmt vet ## Build manager binary.
	go build -o bin/manager cmd/main.go

.PHONY: run
run: manifests generate fmt vet ## Run a controller from your host.
	go run ./cmd/main.go

.PHONY: docker-build
docker-build: ## Build docker image with the manager.
	#cd config/manager && $(KUSTOMIZE) edit set image controller=hub.qucheng.com/platform/cne-operator:${TAG}
	docker build --build-arg VERSION=$(BUILD_VERSION) \
  				--build-arg GIT_COMMIT=$(commit_id) \
  				--build-arg GIT_BRANCH=$(branch_name) \
  				-t hub.qucheng.com/platform/cne-operator:$(TAG) \
  				-f Dockerfile .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push hub.qucheng.com/platform/qucheng:$(TAG)

operator: docker-build docker-push

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply  --kubeconfig ${KUBECFG} -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete  --kubeconfig ${KUBECFG} --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply  --kubeconfig ${KUBECFG} -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete  --kubeconfig ${KUBECFG} --ignore-not-found=$(ignore-not-found) -f -

.PHONY: genclient
genclient: ## Gen Client Code
	hack/genclient.sh

.PHONY: local
local: manifests kustomize docker ## Run local manager.
	$(KUSTOMIZE) build config/default > hack/deploy/deploy.yaml

local-crd: manifests kustomize ## gen crd
	$(KUSTOMIZE) build config/crd > hack/deploy/crd.yaml

.PHONY: local-k8s
local-k8s: ## setup kind k8s
	hack/kind/setup.sh

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

## Tool Binaries
KUSTOMIZE ?= $(LOCALBIN)/kustomize
CONTROLLER_GEN ?= $(LOCALBIN)/controller-gen
ENVTEST ?= $(LOCALBIN)/setup-envtest

## Tool Versions
KUSTOMIZE_VERSION ?= v3.8.7
CONTROLLER_TOOLS_VERSION ?= v0.9.0

KUSTOMIZE_INSTALL_SCRIPT ?= "https://raw.githubusercontent.com/kubernetes-sigs/kustomize/master/hack/install_kustomize.sh"
.PHONY: kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	curl -s $(KUSTOMIZE_INSTALL_SCRIPT) | bash -s -- $(subst v,,$(KUSTOMIZE_VERSION)) $(LOCALBIN)

.PHONY: controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
$(CONTROLLER_GEN): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest
