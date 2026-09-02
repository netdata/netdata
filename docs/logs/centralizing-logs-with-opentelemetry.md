# Centralizing Logs with OpenTelemetry

You centralize a log source by running an OpenTelemetry Collector where the logs are produced and pointing it at the
OTLP endpoint of a Netdata Agent. That Agent stores the logs in Netdata's log store, with retention per tenant and
optional offloading to object storage, and shows them in its Logs tab under the `otel-logs` source. Centralize the
sources that must outlive their node, need retention beyond the node's disk, or have no OS log store, such as
Kubernetes; leave the rest managed in place, where they cost nothing extra. See
[Logs Management](/docs/category-overview-pages/working-with-logs.md) for the decision per source.

Two things to set up:

1. **The receiving Agent.** Any Netdata Agent with the OpenTelemetry plugin (see the availability note in
   [OTLP Ingestion](/docs/opentelemetry/otlp-ingestion.md)). Bind its OTLP endpoint beyond loopback with TLS or mutual TLS, and enable tenant selection when different
   sender groups need their own retention. See
   [Securing the OTLP Endpoint](/docs/opentelemetry/securing-the-otlp-endpoint.md) and, for
   retention and offloading to object storage, [Log Storage and Retention](/docs/logs/log-storage-and-retention.md).
2. **The senders.** One OpenTelemetry Collector per node or cluster, with a persistent queue and the receiver for the
   source. The recipes are in [Collect Logs with OpenTelemetry Collector](/docs/opentelemetry/logs-collection.md):
   [systemd journal](/docs/opentelemetry/logs-collection.md#systemd-journal),
   [application log files](/docs/opentelemetry/logs-collection.md#application-log-files),
   [Windows event channels](/docs/opentelemetry/logs-collection.md#windows-event-channels),
   [Kubernetes and containers](/docs/opentelemetry/logs-collection.md#kubernetes-and-containers), and the
   [macOS unified log](/docs/opentelemetry/logs-collection.md#macos-unified-log). Network devices sending syslog use
   the [syslog receiver](/docs/npm/syslog/README.md).

SNMP traps and network flows need no Collector: Netdata receives them directly and writes journal-compatible files on
the receiving node; see [SNMP Trap Logs](/docs/logs/snmp-trap-logs.md) and [Network Flows](/docs/logs/network-flows.md).

To normalize or drop records before they reach Netdata, see [Transformations](/docs/opentelemetry/transformations.md);
to alert on a log pattern, derive a metric with [Logs-to-Metrics](/docs/opentelemetry/logs-to-metrics.md).
