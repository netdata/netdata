# otel-streams

Streams real-world log events to otel-plugin via OTLP gRPC on `:4317`.

## Binaries

| Binary | Source | Type |
|---|---|---|
| `certstream` | Certificate Transparency Log (WebSocket) | Live stream |
| `jetstream` | Bluesky Jetstream firehose (WebSocket) | Live stream |
| `github` | GH Archive (HTTP + gzip) | Historical replay |
| `synth` | Deterministic in-process generator | Synthetic test corpus |

### Common options

All binaries share these flags:

```
--otel-endpoint <ADDR>      OTel gRPC endpoint  [default: http://127.0.0.1:4317]
--batch-size <N>            Max events per gRPC request  [default: 100]
--flush-interval-ms <MS>    Max ms before flushing a partial batch  [default: 1000]
--tenant-id <ID>            Tenant ID sent via X-Scope-OrgID gRPC header
--log-level <LEVEL>         Tracing log level  [default: info]
```

---

## certstream

Streams Certificate Transparency Log events from a
[certstream-server-go](https://github.com/d-Rickyy-b/certstream-server-go)
WebSocket.

### Prerequisites

```bash
docker run -d --rm --name certstream-server -p 8080:8080 0rickyy0/certstream-server-go
```

### Run

```bash
cargo run --release -p otel-streams --bin certstream
```

Source-specific: `--certstream-url <URL>` [default: `ws://127.0.0.1:8080/`].

---

## jetstream

Streams Bluesky Jetstream events (posts, likes, follows, etc.) from the
public firehose.

### Run

```bash
cargo run --release -p otel-streams --bin jetstream
```

### Filtering by collection

```bash
cargo run --release -p otel-streams --bin jetstream \
    --collections app.bsky.feed.post,app.bsky.feed.like
```

Source-specific: `--jetstream-url <URL>` [default: `wss://jetstream2.us-east.bsky.network/subscribe`], `--collections <LIST>`.

---

## github

Replays GitHub event archives from [GH Archive](https://www.gharchive.org/) as
OTel logs. Downloads hourly `.json.gz` files and replays them at a configurable
rate. Defaults to the previous UTC hour and advances forward indefinitely.

### Run

```bash
cargo run --release -p otel-streams --bin github
```

### Specific hour

```bash
cargo run --release -p otel-streams --bin github -- --start 2024-06-01-12
```

Source-specific: `--start <YYYY-MM-DD-H>` [default: previous UTC hour], `--rate <N>` [default: 100, 0 = unlimited].

---

## synth

Generates a **deterministic, reproducible** batch of OTLP log records and sends
them through the same `Sender` / `build_export_request` path as the live
sources. Unlike the live streams, output is a pure function of the parameters
(no RNG, no clock inside generation), so it produces known-exact corpora for
verifying the otel-logs subsystem — e.g. forcing WAL rotation/eviction over a
known count, or exercising low/mid/high field-cardinality tiers.

Each record carries: a monotonic timestamp; a cycled severity (low-cardinality
`level`); `host`/`code` over `--field-cardinality` distinct values (mid); and a
unique `seq` (high).

### Run

```bash
cargo run --release -p otel-streams --bin synth -- --count 25 --field-cardinality 4
```

Source-specific: `--count <N>` [default: 100], `--field-cardinality <N>` [default: 100], `--spacing-nanos <NS>` [default: 1e9], `--start-time-nanos <NS>` [default: now − count·spacing], `--seed <N>` [default: 0].
