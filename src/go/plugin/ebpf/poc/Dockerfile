FROM golang:1.27.1-trixie

RUN apt-get update && apt-get install -y --no-install-recommends \
    libbpf-dev libelf-dev zlib1g-dev bpftool python3 ca-certificates curl xz-utils \
    && rm -rf /var/lib/apt/lists/*

# Match packaging/cmake/Modules/NetdataEBPFCORE.cmake; no Netdata Agent build.
RUN mkdir -p /opt/ebpf-core \
    && curl -fsSL https://github.com/netdata/ebpf-co-re/releases/download/v1.7.0.2/netdata-ebpf-co-re-glibc-v1.7.0.2.tar.xz -o /tmp/core.tar.xz \
    && echo '846566c46a9b5ab356ef5ed2d2ee7fc222c6014a1ac6c5b3a9905357bddab3ce  /tmp/core.tar.xz' | sha256sum -c - \
    && tar -xJf /tmp/core.tar.xz -C /opt/ebpf-core \
    && rm /tmp/core.tar.xz

WORKDIR /src/src/go
COPY src/go/go.mod src/go/go.sum ./
COPY src/collectors/ebpf.plugin/ebpfgo.plugin/go.mod /src/src/collectors/ebpf.plugin/ebpfgo.plugin/go.mod
COPY src/go ./
COPY src/collectors/ebpf.plugin/ebpfgo.plugin /src/src/collectors/ebpf.plugin/ebpfgo.plugin

ENV CGO_ENABLED=1 CGO_LDFLAGS="-lbpf -lelf -lz"
# First compile the native backend without skeletons, matching its legacy-only build.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test -race -count=1 -tags netdata_ebpf_libbpf ./plugin/ebpf/... ./cmd/ebpfpocplugin
ENV CGO_CFLAGS="-I/opt/ebpf-core/includes"
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go test -race -count=1 -tags netdata_ebpf_libbpf ./plugin/ebpf/... ./cmd/ebpfpocplugin \
    && go vet -tags netdata_ebpf_libbpf ./plugin/ebpf/... ./cmd/ebpfpocplugin \
    && go build -tags netdata_ebpf_libbpf -o /usr/local/bin/ebpf-poc.plugin ./cmd/ebpfpocplugin

# Keep the original executable buildable through the local module relationship.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    cd ../collectors/ebpf.plugin/ebpfgo.plugin \
    && go test -count=1 ./... \
    && go build -tags netdata_ebpf_libbpf -o /tmp/ebpf-go-original.plugin .

CMD ["python3", "plugin/ebpf/poc/smoke.py"]
