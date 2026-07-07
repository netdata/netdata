# otel-streams

A developer tool for feeding the otel-plugin with OpenTelemetry logs over
OTLP gRPC (default endpoint `http://127.0.0.1:4317`). It generates both
real-world log streams — by tapping live public event sources — and
synthetic, fully deterministic corpora, so the otel-logs ingestion, WAL,
and query path can be exercised against realistic data as well as
known-exact fixtures.

## Streams

- **certstream** — real, live. Certificate Transparency Log events from a
  certstream-server WebSocket.
- **jetstream** — real, live. The Bluesky Jetstream firehose (posts, likes,
  follows, and similar events), optionally filtered by collection.
- **ris** — real, live. The RIPE RIS Live BGP firehose (routing updates from
  the global RRC route collectors, ~4k messages/second), optionally filtered by
  collector (`--host`) or BGP message type (`--type`) to throttle volume.
- **wikimedia** — real, live. The Wikimedia EventStreams `recentchange` feed
  (edits, page creations, and log actions across all wikis) over SSE, with
  several low/mid-cardinality categorical fields (change type, wiki, namespace,
  bot/minor flags, log type/action) promoted to attributes.
- **github** — real, historical replay. Hourly GH Archive event archives,
  downloaded and replayed at a configurable rate.
- **synth** — synthetic. A deterministic, reproducible generator whose
  output is a pure function of its parameters (no RNG, no clock inside
  generation), producing known-exact corpora — for example forcing WAL
  rotation/eviction over a known record count, or exercising low/mid/high
  field-cardinality tiers.
