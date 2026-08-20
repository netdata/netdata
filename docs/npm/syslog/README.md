<!--
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/syslog/README.md"
sidebar_label: "Syslog from Network Devices"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Syslog from Network Devices"
description: "Ingest syslog from network devices through the OpenTelemetry Collector and explore it in the Logs tab."
learn_link: "https://learn.netdata.cloud/docs/network-performance-monitoring/syslog-from-network-devices"
slug: "/network-performance-monitoring/syslog-from-network-devices"
-->






<!-- markdownlint-disable-file -->

# Syslog from Network Devices

Netdata ingests the syslog your network devices emit — configuration changes, authentication events, link state, hardware alarms — and stores it as structured logs you can search and filter alongside each device's metrics, topology, and traps.

## How it reaches Netdata

Netdata does not listen for syslog directly. Your devices send syslog to an **OpenTelemetry Collector** with a syslog receiver, which parses each message (RFC 3164 or 5424), normalizes the fields, and forwards them to the Netdata Agent over OTLP; the Agent indexes them as structured OpenTelemetry logs for the **Logs** tab. Because the pipeline is the OpenTelemetry Collector, you can filter, transform, and enrich messages — or derive metrics from them — before they reach Netdata.

## What you get

- **Searchable device logs** — by device, severity, facility, and message content.
- **Alongside everything else** — run the collector against the same Agent that polls the devices, and their logs sit beside their metrics, topology, and traps.
- **Retained on your terms** — write-ahead logs rotate and indexed files follow the retention limits you set.

## Where to start

- Netdata publishes ready-to-use OpenTelemetry Collector configurations for syslog. The entry in this section walks through the collector configuration and what to expect in the Logs tab.
