<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/snmp-traps/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'snmp', 'trap', 'inform', 'network events', 'logs', 'netops', 'secops', 'snmpv3', 'otlp', 'siem']
endmeta-->

<!-- markdownlint-disable-file -->

# SNMP Traps

Netdata listens for SNMP Trap and INFORM notifications from your network devices, decodes them into named, categorized, structured log entries, summarizes receiver activity as metrics, and can forward the same events to a SIEM or log system.

This section is for NetOps, NOC, SRE, SecOps, and MSP operators who need to know which network events a device reported, whether the receiver is healthy, and when trap data should be queried or forwarded.

![SNMP trap events in the Netdata Logs UI](https://www.netdata.cloud/img/network/snmp-trap-logs.png)

Decoded trap and INFORM events in the Netdata Logs tab — named, categorized, and searchable, with severity and source.

## What trap data is

An SNMP trap is an *asynchronous event notification*. A device sends one when it decides something happened — an interface changed state, an authentication failed, a configuration changed, a sensor crossed a threshold. It is one telemetry leg among several: traps and streaming telemetry carry events, polling confirms current state, syslog carries narrative, flow carries traffic. Traps are indispensable because every device supports them and they catch transient transitions that polling would miss between intervals.

Three facts decide how you read every trap, and they are how trap monitoring works everywhere — not Netdata specifics:

- **Traps are pushed, and they are lossy.** A device sends a trap once, over UDP, with no retransmit and no acknowledgement (INFORMs are the acknowledged exception). A trap that is dropped in transit, blocked by a firewall, or lost to a full buffer leaves no trace. So **silence is never proof of health** — see [What you cannot answer](#what-you-cannot-answer).
- **Most traps are noise.** The large majority of traps are low-severity informational events. The value is not in any single trap but in the *stream* — rates, repeats, clusters, and the ones you have never seen before.
- **Traps need a meaning layer.** Without a profile (a compiled MIB), a trap is an opaque numeric OID. Netdata's [trap profiles](/docs/npm/snmp-traps/trap-profiles.md) are that layer: they turn OIDs into names, categories, severities, and labeled fields.

## What you can answer

- Did this device, listener, or site send traps — and which trap names, categories, and severities are active now?
- Which devices are reporting state changes, authentication failures, configuration changes, security, or license events?
- Is a surge mostly repeated duplicates, a rate-limited flap, decode errors, or many real distinct events?
- Is the receiver healthy, or is it dropping, suppressing, or failing to write traps?
- Are traps also reaching my SIEM when OTLP export is enabled?

## What you cannot answer

- **Is the device healthy because no traps arrived?** No. Silence is ambiguous: the device may be quiet, not configured to send traps, blocked by a firewall or allowlist, using the wrong credentials — or the receiver may be dropping. The trap path is independent of the device's data plane, and a hard-faulted device may never get to send a trap at all. **Pair every critical device with polling; traps are the ceiling, polling is the floor.**
- **Can traps replace polling metrics?** No. Traps are events, not a current-state stream. Use Netdata's metrics collectors for continuous device state.
- **Is high trap volume the same as a severe incident?** No. It can be a flap, a duplicate storm, or a retransmission pattern. Read the name, category, severity, source, and dedup before reacting.
- **Did an event definitely not happen?** Not from trap absence alone. Reachability, credentials, allowlists, and receiver health all gate delivery.

If those are your questions, use traps together with polling, device configuration, and receiver health.

## Two things to know on day one

These two truths prevent the most common first-week mistakes.

### Silence is not health

Netdata does not create listener jobs or configure devices for you. You create a listener job, then point devices at it. Until you have confirmed the first trap arrived — and you are watching receiver metrics — a quiet receiver tells you nothing: it could be a quiet network, an unconfigured device, a blocked port, wrong credentials, or a broken receiver. Confirm first receipt before you trust silence.

### Trap count is only the start

A count tells you the receiver saw activity, not whether it matters. Use the resolved trap name, category, severity, source, and varbinds to understand meaning, then check dedup, drops, decode errors, and export errors to confirm the receiver handled the traffic cleanly.

## What ships with the collector

- **A listener** for SNMPv1, SNMPv2c, and SNMPv3 Trap and INFORM notifications, with USM authentication and privacy and SNMPv3 engine-ID controls. Netdata acknowledges every INFORM.
- **A trap profile catalogue** — 800+ stock vendor profiles covering 6,000+ MIBs — that decodes OIDs into names, categories, severities, and labeled varbinds out of the box. Per-OID overrides and custom profiles cover the rest.
- **Local journal storage** (default, Linux): structured trap entries you query with the Netdata Logs UI or `journalctl`.
- **Optional OTLP/gRPC export** to forward traps as log records to a SIEM or log pipeline. At least one output — journal or OTLP — must be enabled.
- **Receiver self-metrics and alerts**: a pipeline funnel, events by category and severity, processing errors, deduplication, and per-source health.
- **A closed taxonomy** of 8 categories (`state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, `unknown`) and 8 severities (`emerg` … `debug`), so categories and severities mean the same thing across every device.

Trap data can carry sensitive operational values (hostnames, locations, usernames, vendor text); treat it as sensitive when querying and forwarding.

## Where to start

Pick the page that matches your situation:

- **You're setting up for the first time** — [Installation](/docs/npm/snmp-traps/installation.md), then [Quick Start](/docs/npm/snmp-traps/quick-start.md) to prove the first trap arrives, then [Configuration](/docs/npm/snmp-traps/configuration.md) to harden the listener.
- **You want decoded traps to mean something** — [Trap Profiles](/docs/npm/snmp-traps/trap-profiles.md) turn OIDs into names and severities, and [Enrichment](/docs/npm/snmp-traps/enrichment.md) adds device, vendor, and topology context.
- **You have traps and need to find or triage one** — [Investigation Playbooks](/docs/npm/snmp-traps/investigation-playbooks.md), then [Usage and Output](/docs/npm/snmp-traps/usage-and-output.md) to read a row and the [Field Reference](/docs/npm/snmp-traps/field-reference.md) to look up any field.
- **You want to trust what you see** — [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md), and avoid the common mistakes in [Anti-patterns](/docs/npm/snmp-traps/anti-patterns.md).
- **You want to search or export trap data** — [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md).
- **You want to alert or forward** — [Alerts](/docs/npm/snmp-traps/alerts.md) for receiver health, [Metrics](/docs/npm/snmp-traps/metrics.md) for the signals they watch, and [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md) to send events downstream.
- **You're planning for scale** — [Sizing and Capacity](/docs/npm/snmp-traps/sizing-and-capacity.md).
- **Something's wrong** — [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md).
