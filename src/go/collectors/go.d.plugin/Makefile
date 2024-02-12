# SPDX-License-Identifier: GPL-3.0-or-later

DEV_MODULES := all

all: download vet test build

.PHONY: help
help:
	@grep -E '^[a-zA-Z0-9_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

.PHONY: download
download: ## Download go modules
	go mod download

.PHONY: build
build: clean ## Build package
	hack/go-build.sh

.PHONY: clean
clean:
	rm -rf bin vendor

.PHONY: check
check: fmt vet ## Run static code analysis


.PHONY: test
test: ## Run tests
	go test ./... -race -cover -covermode=atomic

.PHONY: fmt
fmt:
	hack/go-fmt.sh .

.PHONY: vet
vet:
	go vet ./...

.PHONY: release
release: clean download ## Create all release artifacts
	hack/go-build.sh all
	hack/go-build.sh configs
	hack/go-build.sh vendor
	cd bin && sha256sum -b * >"sha256sums.txt"

.PHONY: dev
dev: dev-build dev-up ## Launch development build

dev-build:
	docker-compose build

dev-up:
	docker-compose up -d --remove-orphans

.PHONY: dev-exec
dev-exec: ## Get into development environment
	docker-compose exec netdata bash

dev-log:
	docker-compose logs -f netdata

dev-run: ## Run go.d.plugin inside development environment
	go run github.com/netdata/go.d.plugin/cmd/godplugin -d -c conf.d

dev-mock: ## Run go.d.plugin inside development environment with mock config
	go run github.com/netdata/go.d.plugin/cmd/godplugin -d -c ./mocks/conf.d -m $(DEV_MODULES)
