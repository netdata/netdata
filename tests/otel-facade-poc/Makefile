.PHONY: build test integration linux agent-smoke

IMAGE ?= netdata/netdata:edge

build:
	mkdir -p .build
	go run go.opentelemetry.io/collector/cmd/builder@v0.157.0 --config builder-config.yaml

test:
	go test -race ./...
	go vet ./...

integration: build
	OTEL_POC_BINARY="$(CURDIR)/.build/collector/otel-facade" go test -race -count=1 -run TestDistribution ./...

linux: build
	cd .build/collector && CGO_ENABLED=0 GOOS=linux go build -o ../otel-facade-linux .

agent-smoke: linux
	python3 agent_smoke.py --image "$(IMAGE)"
