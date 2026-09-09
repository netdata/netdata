<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/device-metrics/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "Network Performance Monitoring/Device Metrics"
keywords: ['snmp', 'network devices', 'router', 'switch', 'firewall', 'discovery', 'interface', 'overview']
endmeta-->

<!-- markdownlint-disable-file -->

# Device Metrics

Netdata monitors your network devices over SNMP — routers, switches, firewalls, access points, load balancers, UPS and PDU units. It recognizes each device's model automatically and collects interface, health, and vendor-specific metrics, with no OID lists to build and no per-device dashboards to design.

![Network devices on the Nodes tab](https://www.netdata.cloud/img/dashboard-screens/nodes-tab-network-devices.png)

Each SNMP device becomes its own node — model-recognized, with live interface, health, and vendor metrics.

## What you can monitor

- **Interfaces** — traffic, errors, discards, and operational status, per interface.
- **Device health** — control-plane CPU and memory, and environmental sensors (temperature, fans, power supplies) where the device's profile exposes them.
- **Vendor-specific metrics** — hardware, protocol, and feature metrics from the matched vendor profile, plus BGP peers and license state where the device reports them.
- **Reachability** — optional ICMP latency and loss alongside every device.

Each device becomes its own node, with live charts, labels, and alerts.

## Model recognition

Netdata matches each device to a profile by its `sysObjectID`, from a large built-in profile library. It collects standard interface and system metrics for any device that answers SNMP, and vendor-specific metrics where a profile matches; you can add custom profiles for anything special. It polls over SNMP v1, v2c, and v3.

## Functions and alerts

- **Live tables** — `snmp:interfaces` (traffic, status, errors, discards), `snmp:bgp-peers` (per-peer state and recency), and `snmp:licenses`.
- **Stock alerts** — license expiry and BGP session health.

## Where to start

- **Your first device** — [Quick Start](/docs/npm/device-metrics/quick-start.md).
- **Credentials, SNMPv3, many devices, discovery** — [Configuration](/docs/npm/device-metrics/configuration.md).
- **Understand or extend model recognition** — [SNMP Profile Format](/src/go/plugin/go.d/collector/snmp/profile-format.md).
- **Many devices or many sites** — [Sizing and Scaling](/docs/npm/device-metrics/sizing-and-scaling.md).
- **Trust the data** — [Validation](/docs/npm/device-metrics/validation.md); avoid the common mistakes in [Anti-patterns](/docs/npm/device-metrics/anti-patterns.md).
- **Something's off** — [Troubleshooting](/docs/npm/device-metrics/troubleshooting.md).
