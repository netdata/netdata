<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/syslog-pointer.md"
sidebar_label: "Syslog"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring"
keywords: ['syslog', 'network devices', 'opentelemetry', 'npm']
endmeta-->

# Syslog

Netdata ingests syslog from network devices through an OpenTelemetry Collector: the Collector listens for syslog over
UDP or TCP, parses RFC 3164 or RFC 5424 records, and forwards them over OTLP to a Netdata Agent, which stores them in
its indexed log store and shows them in the Logs tab under the `otel-logs` source — next to the same devices' SNMP
metrics, traps, and flows.

The setup is documented in the OpenTelemetry section:

- [Syslog from Network Devices](/docs/npm/syslog/README.md) — what to expect in the Logs tab.
- [OpenTelemetry Collector Setup](/docs/npm/syslog/otel-collector.md) — a working receiver configuration and how to
  make it durable.
- [Securing the OTLP Endpoint](/docs/opentelemetry/securing-the-otlp-endpoint.md) — TLS and network controls for the
  receiving Agent.

Related device data under Network Performance Monitoring: [SNMP Traps](/docs/npm/snmp-traps/README.md) and
[Network Flows](/docs/npm/network-flows/README.md).
