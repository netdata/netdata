<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/syslog/otel-collector.md"
sidebar_label: "OpenTelemetry Collector Setup"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Syslog from Network Devices"
keywords: ['syslog', 'opentelemetry', 'otel collector', 'otlp', 'configuration', 'setup']
endmeta-->

<!-- markdownlint-disable-file -->

# OpenTelemetry Collector Setup

Network devices send syslog; an OpenTelemetry Collector receives it, normalizes it, and forwards it to Netdata over OTLP. This page walks through a working configuration.

## What you need

- A Netdata Agent — its OTLP endpoint listens on `127.0.0.1:4317` by default.
- An [OpenTelemetry Collector Contrib](https://github.com/open-telemetry/opentelemetry-collector-releases) binary on the network, typically on the same host as the devices' hub.
- Devices configured to send syslog to the collector.

## A working configuration

This listens for RFC 3164 (BSD) syslog over UDP, normalizes the fields to OpenTelemetry conventions, and exports to Netdata:

```yaml
receivers:
  syslog:
    location: "Europe/Athens"        # set your timezone — BSD syslog carries none
    udp:
      listen_address: "0.0.0.0:53514"
    protocol: rfc3164                 # switch to rfc5424 if your devices use it

processors:
  transform/syslog:
    error_mode: ignore
    log_statements:
      - set(resource.attributes["host.name"], log.attributes["hostname"])
      - set(resource.attributes["syslog.appname"], log.attributes["appname"])

exporters:
  otlp:
    endpoint: localhost:4317
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [syslog]
      processors: [transform/syslog]
      exporters: [otlp]
```

Start it with `otelcol-contrib --config ./otelcol.yaml`, then point your devices' syslog at the collector's host on UDP `53514`.

To receive over **TCP** instead (common for RFC 5424), replace the `udp:` block with `tcp:`:

```yaml
    tcp:
      listen_address: "0.0.0.0:53514"
```

## Confirm it's flowing

Open the **Logs** tab in Netdata, select the hub node, and search for one of your devices by hostname. Within a poll cycle you should see its syslog messages with their parsed severity and facility. No messages usually means the devices aren't pointed at the collector, or a firewall is dropping the listener's port.

## Make it durable

For production, add a file-backed sending queue so a brief Netdata restart doesn't drop messages in flight. Netdata publishes ready-to-use **simple** and **robust** configurations in the [otelcol-cookbook](https://github.com/netdata/otelcol-cookbook) `syslog-ingest` recipe — start from those rather than building the pipeline from scratch.

## What you get

Each message arrives in Netdata as a structured journal log, searchable and filterable in the **Logs** tab. The configuration above produces these fields:

- **`resource.attributes.host.name`** — the device that sent the message.
- **`resource.attributes.syslog.appname`** — the application name.
- **`log.severity_text`** / **`log.severity_number`** — the parsed severity.
- **`log.attributes.facility_text`** (the name, e.g. `local0`) and **`log.attributes.facility`** (the numeric code) — the syslog facility.
- **`log.body`** (the line as received) and **`log.attributes.message`** (the parsed message text).

These are the fields the inline configuration above produces; a production configuration such as the cookbook recipe may normalize some of these names, so check its output when you build saved searches against it. Search or filter on any of them — for example, `resource.attributes.host.name` to focus on one switch, or `log.severity_text` to surface only the severities you care about.

## What's next

- [Overview](/docs/npm/syslog/README.md) — how syslog fits the hub.
