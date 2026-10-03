# OTLP/JSON example payloads

`logs.json`, `metrics.json` and `trace.json` are copied unmodified from the official OTLP JSON
request examples:

- Source: `open-telemetry/opentelemetry-proto @ b3f7558`, `examples/`
- License: Apache License 2.0 (the opentelemetry-proto repository license)

The OTLP/HTTP receiver tests (`src/otlp_json.rs`) decode them to pin that spec-conformant JSON
survives decoding with every metric and its values intact.
