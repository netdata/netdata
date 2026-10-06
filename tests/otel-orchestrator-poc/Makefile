.PHONY: build test integration linux-amd64 linux-arm64 installer-test agent-smoke

IMAGE ?= netdata/netdata:edge
SMOKE_ARCH ?= amd64

build:
	python3 build.py build

linux-amd64 linux-arm64:
	python3 build.py $@

test:
	go test -race ./...
	go vet ./...
	cd ../../src/go && go test -race ./plugin/otelcolpoc ./cmd/otelcolpocplugin
	cd ../../src/go && go vet ./plugin/otelcolpoc ./cmd/otelcolpocplugin

installer-test:
	python3 installer_test.py -v

integration: build
	cd ../../src/go && OTEL_POC_WORKER="$(CURDIR)/.build/otel-worker" go test -race -count=1 -ldflags '-X "github.com/netdata/netdata/go/plugins/pkg/buildinfo.NetdataBinDir=$(CURDIR)/.build/bin"' ./plugin/otelcolpoc
	OTEL_POC_BINARY="$(CURDIR)/.build/otel-orchestrator.plugin" go test -race -count=1 -run TestOrchestrator ./...

agent-smoke: linux-$(SMOKE_ARCH)
	python3 agent_smoke.py --binary-dir ".build/linux-$(SMOKE_ARCH)" --image "$(IMAGE)"
