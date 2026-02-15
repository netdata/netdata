# certstream-otel-bridge

Bridges Certificate Transparency Log events to OTel logs via gRPC. Connects to a [certstream-server-go](https://github.com/d-Rickyy-b/certstream-server-go) WebSocket and streams every certificate as an OTel LogRecord to otel-plugin on `:4317`.

## Prerequisites

The public `wss://certstream.calidog.io/` endpoint is defunct. You need a local certstream-server-go instance:

```bash
docker run -d --name certstream-server -p 8080:8080 0rickyy0/certstream-server-go
```

Verify it's running:

```bash
docker logs certstream-server --tail 5
```

You should see lines like `Processed 1000 entries`.

## Usage

```bash
cargo run --release -p certstream-otel-bridge -- --certstream-url ws://127.0.0.1:8080/
```

### Options

```
--certstream-url <URL>      WebSocket endpoint  [default: wss://certstream.calidog.io/]
--otel-endpoint <ADDR>      OTel gRPC endpoint  [default: http://127.0.0.1:4317]
--batch-size <N>            Max events per gRPC request  [default: 100]
--flush-interval-ms <MS>    Max ms before flushing a partial batch  [default: 1000]
--log-level <LEVEL>         Tracing log level  [default: info]
```

### Example with larger batches

```bash
cargo run --release -p certstream-otel-bridge -- \
    --certstream-url ws://127.0.0.1:8080/ \
    --batch-size 1000
```
