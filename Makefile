# Copyright (c) 2025, NVIDIA CORPORATION.  All rights reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Main Makefile for NVIDIA Device API

# ==============================================================================
# Configuration
# ==============================================================================

SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

VERSION_PKG = github.com/nvidia/nvsentinel/pkg/util/version
GIT_VERSION := $(shell git describe --tags --always --dirty)
GIT_COMMIT  := $(shell git rev-parse HEAD)
BUILD_DATE  := $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -X $(VERSION_PKG).GitVersion=$(GIT_VERSION) \
           -X $(VERSION_PKG).GitCommit=$(GIT_COMMIT) \
           -X $(VERSION_PKG).BuildDate=$(BUILD_DATE)

# ==============================================================================
# Targets
# ==============================================================================

.PHONY: all
all: code-gen test build ## Run code generation, test, and build for all modules.

##@ General

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: code-gen
code-gen: ## Run code generation.
	./hack/update-codegen.sh
	go mod tidy

.PHONY: verify-codegen
verify-codegen: code-gen ## Verify generated code is up-to-date.
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "ERROR: Generated code is out of date. Run 'make code-gen'."; \
		git status --porcelain; \
		git --no-pager diff; \
		exit 1; \
	fi

.PHONY: tidy
tidy: ## Run go mod tidy
	go mod tidy

##@ Build & Test

.PHONY: build
build: ## Build the device-apiserver binary.
	go build -ldflags "$(LDFLAGS)" -o bin/device-apiserver ./cmd/device-apiserver

.PHONY: test
test: ## Run unit tests.
	GOTOOLCHAIN=go1.25.5+auto go test -v $$(go list ./... | grep -vE '/pkg/client-go/(client|informers|listers)|/internal/generated/|/test/integration/|/examples/') -cover cover.out

.PHONY: test-integration
test-integration: ## Run integration tests.
	go test -v ./test/integration/...

.PHONY: lint
lint: ## Run golangci-lint.
	golangci-lint run ./...

.PHONY: lint-changed
lint-changed: REVISION ?= HEAD~1
lint-changed: ## Run golangci-lint on changed files only.
	golangci-lint run --new-from-rev=$(REVISION) ./...

.PHONY: clean
clean: ## Remove generated artifacts.
	@echo "Cleaning generated artifacts..."
	rm -rf bin/
	rm -rf internal/generated/
	rm -rf pkg/client-go/client/ pkg/client-go/informers/ pkg/client-go/listers/
	find api/ -name "zz_generated.deepcopy.go" -delete
	find api/ -name "zz_generated.goverter.go" -delete
	rm -f cover.out
