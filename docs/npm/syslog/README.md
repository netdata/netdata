<!-- markdownlint-disable-file -->

# Syslog from Network Devices

Netdata ingests the syslog your network devices emit — configuration changes, authentication events, link state, hardware alarms — and stores it as structured logs you can search and filter alongside each device's metrics, topology, and traps.

## How it reaches Netdata

Netdata does not listen for syslog directly. Your devices send syslog to an **OpenTelemetry Collector** with a syslog receiver, which parses each message (RFC 3164 or 5424), normalizes the fields, and forwards them to the Netdata Agent over OTLP; the Agent stores them as structured journal logs in the **Logs** tab. Because the pipeline is the OpenTelemetry Collector, you can filter, transform, and enrich messages — or derive metrics from them — before they reach Netdata.

## What you get

- **Searchable device logs** — by device, severity, facility, and message content.
- **Alongside everything else** — run the collector against the same Agent that polls the devices, and their logs sit beside their metrics, topology, and traps.
- **Retained on your terms** — stored in journal files with the rotation and retention you set.

## Where to start

- Netdata publishes ready-to-use OpenTelemetry Collector configurations for syslog. The entry in this section walks through the collector configuration and what to expect in the Logs tab.
