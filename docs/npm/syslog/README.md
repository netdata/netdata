<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/syslog/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Syslog from Network Devices"
keywords: ['syslog', 'opentelemetry', 'otel', 'otlp', 'network devices', 'logs', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Syslog from Network Devices

Netdata ingests the syslog your network devices emit — configuration changes, authentication events, link state, hardware alarms — and stores it as structured logs you can search, filter, and keep alongside the rest of each device's signals.

This section is for the teams who want their routers', switches', and firewalls' logs in the same place as their metrics, topology, and traps — searchable, retained, and ready when an incident needs them.

## How it works

Network devices send syslog. Netdata receives it through the **OpenTelemetry Collector** — the open, vendor-neutral telemetry layer Netdata builds on:

1. You run an OpenTelemetry Collector with a **syslog receiver**, typically on the same host as the devices' hub.
2. It parses each message (RFC 3164 or 5424), normalizes the fields, and forwards them to the Netdata Agent over OTLP.
3. The Agent stores them as structured journal logs, explorable in the **Logs** tab.

Because the pipeline is the OpenTelemetry Collector, you also get its full processing power — filter noisy sources, transform and enrich messages, even derive metrics from log patterns — before the logs ever reach Netdata.

## What you get

- **Searchable device logs** — by device, severity, facility, and message content.
- **Together with everything else** — run the collector against the same hub that polls the devices, and their logs sit beside their metrics, topology, and traps.
- **Retained on your terms** — stored in journal files with the rotation and retention you set.

## Set it up

Netdata publishes ready-to-use OpenTelemetry Collector configurations for syslog — a simple one to get started, and a more robust one with a durable sending queue. Point your devices at the collector, point the collector at your Agent, and the logs flow.

## Where to start

- The entry in this section walks through the collector configuration and what to expect in the Logs tab.
