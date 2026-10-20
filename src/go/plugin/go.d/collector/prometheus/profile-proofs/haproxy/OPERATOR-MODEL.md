# HAProxy PromEx operator model

The profile models HAProxy's built-in PromEx exporter compiled with `USE_PROMEX=1`. The source contract is grounded in
`haproxy/haproxy` v3.2.0 and covers the complete default endpoint without the optional `extra-counters` module.

## Operator hierarchy

- **Process:** worker runtime, resource limits, scheduler activity, SSL, compression, memory, and errors.
- **Frontends and listeners:** ingress status, sessions, connections, HTTP activity, traffic, limits, and failures.
- **Backends and servers:** routing status, queues, timings, health checks, capacity, traffic, and failures.
- **Resolvers:** cumulative DNS query outcomes per resolver and nameserver.
- **Stick tables:** configured capacity and current entry use per table and key type.

Frontend, listener, backend, server, resolver, nameserver, and stick-table identities come from bounded HAProxy
configuration. Closed status and HTTP response-code labels become chart dimensions; they never become chart identity.

## Source/runtime mismatches

HAProxy preserves historical Prometheus wire types for a few fields. Pipe counts and process compression bytes over the
last second are counters on the wire but current values, while resolver module counters are gauges on the wire but
cumulative. The profile carries explicit algorithms only for those mismatches; every other dimension inherits the runtime
metric-kind default.

`haproxy_process_node` and `haproxy_process_description` are constant metadata carriers whose numeric value is always one.
They remain collected but are suppressed from generic fallback. The process start epoch also remains collected but needs
an unavailable age transform, and the deprecated backend aggregate status is suppressed in favor of its exact replacement.
Gauge-valued `haproxy_process_build_info` is rejected by the writer as a conventional information family.
