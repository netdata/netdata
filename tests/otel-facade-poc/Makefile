.PHONY: build test integration linux linux-amd64 agent-smoke

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

linux-amd64: build
	mkdir -p .build/linux-amd64
	cd .build/collector && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o ../linux-amd64/otel-facade .
	cp otel-facade.plugin .build/linux-amd64/otel-facade.plugin

agent-smoke: linux
	python3 agent_smoke.py --image "$(IMAGE)"
