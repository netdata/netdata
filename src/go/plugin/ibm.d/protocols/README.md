# Protocol Clients

This directory contains reusable protocol implementations used by IBM.D modules. Protocols abstract network or API details and return typed data structures so collectors can focus on orchestration.

Highlights:

| Path | Purpose |
|------|---------|
| `openmetrics/` | Generic HTTP client + parser for Prometheus/MicroProfile metrics. |
| `websphere/pmi/` | PerfServlet XML snapshot loader and typed PMI tables. |
| `websphere/jmx/` | Adapter layer on top of the JMX bridge helper. |
| `jmxbridge/` | Process manager for the JSON-over-stdin Java helper (used by WebSphere JMX). |
| `mq/` | IBM MQ PCF/command interfaces. |
| `db2/` | DB2 ODBC helpers (shared with as400 module). |

When creating a new collector: if the target exposes an API that may be reused later, add a protocol here instead of embedding HTTP/ODBC logic directly into the module.

Protocols should:

1. Accept a `context.Context` for cancellation.
2. Return rich Go structs (not `map[string]interface{}`) so collectors get compile-time safety.
3. Handle feature detection gracefully and emit informative errors.

Shared CGO shims live in [`../pkg`](../pkg/README.md).
