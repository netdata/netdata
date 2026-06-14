<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/README.md"
sidebar_label: "Overview"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'snmp', 'trap', 'inform', 'network events', 'logs', 'netops', 'secops', 'snmpv3', 'otlp', 'siem']
endmeta-->

<!-- markdownlint-disable-file -->

# SNMP Traps

The SNMP trap listener runs in `go.d.plugin`. Netdata can listen for SNMP Trap and INFORM notifications from network devices, store them as structured log entries, summarize receiver activity as metrics, and optionally forward the same trap events as OTLP LogRecords.

The built-in trap profile catalogue is a major part of the value: Netdata ships with **800+ stock vendor profiles** covering **6,000+ MIBs** and **150,000+ trap definitions**, so many common network devices can be decoded into meaningful names, categories, severities, and varbind labels without manual MIB work.

This section is for NetOps, NOC, SRE, SecOps, platform teams, and MSP operators who need to know which network events devices reported, whether the receiver pipeline is healthy, and when trap data should be queried or forwarded into another log system.

Trap data has three operator surfaces: direct-journal log entries, receiver self-metrics, and optional OTLP log export. On Linux, direct-journal jobs can be queried locally with `journalctl --directory <per-job-dir>`. In Netdata Cloud, the same jobs appear as SNMP Trap Logs sources (`__logs_sources`) through the embedded `snmp:traps` Function, which requires a Netdata Cloud connection. OTLP-only jobs do not create local journal files or local job sources in the `snmp:traps` Function.

## What trap data is

An SNMP trap is an *asynchronous event notification*. A device sends it when the device decides something significant happened, such as an interface state change, an authentication failure, a configuration change, or a vendor-specific condition.

Netdata accepts:

- **SNMPv1 Trap**
- **SNMPv2c Trap and INFORM**
- **SNMPv3 Trap and INFORM** with USM security levels, including authentication and optional privacy

Netdata sends acknowledgements for all INFORM notifications.

A received trap becomes a structured log entry. That entry can include:

- The listener job that received it
- The source address and transport details
- The SNMP version and PDU type
- The trap OID and resolved trap name
- The profile-assigned category and severity
- Decoded varbind fields and a structured varbind payload
- Deduplication summary fields when repeated traps are suppressed

Trap rows and OTLP exports can include sensitive operational values, such as hostnames, interface descriptions, locations, usernames, and vendor-provided varbind text. Treat trap data as operationally sensitive; see [SNMP trap field reference](/docs/snmp-traps/field-reference.md) and [Configuration](/docs/snmp-traps/configuration.md) for field and export guidance.

Trap profiles provide the meaning layer. The out-of-box profile pack includes 800+ stock vendor profiles and resolves numeric OIDs to names, categories, severities, and varbind labels for common equipment. Unknown or unmatched traps are still stored, but they keep the raw OID and use the `unknown` category until profile coverage or overrides give them more meaning.

## What you can answer

- Did Netdata receive traps from this device, listener, or site?
- Which trap names, OIDs, categories, and severities are active right now?
- Which devices are sending authentication, security, configuration, state-change, license, mobility, diagnostic, or unknown events?
- Is a trap storm mostly repeated duplicates, rate-limited traffic, decode errors, or real distinct events?
- Are direct-journal jobs visible as local log sources through `snmp:traps`?
- Are traps also being exported as OTLP LogRecords when OTLP/gRPC export is enabled?
- Are receiver errors, drops, dedup suppression, source health, or profile-metric diagnostics changing?

## What you cannot answer

- **Is the device healthy because no traps arrived?** No. Traps are emitted only when devices are configured to send them and decide to send them. Silence does not prove health.
- **Can traps replace polling metrics?** No. Traps are event notifications, not a current-state polling stream. Use Netdata's metrics collectors for continuous device state and performance.
- **Is raw trap volume the same as incident severity?** No. A high count can be a flap, duplicate storm, retransmission pattern, or many legitimate events. Inspect category, severity, source, trap name, dedup, drops, and errors.
- **Did an event definitely not happen?** Not from trap absence alone. Device configuration, network reachability, credentials, source allowlists, and receiver health all affect delivery.
- **Is this packet capture?** No. Netdata stores decoded trap fields as logs; it is not a full packet capture workflow.

If those are your questions, use traps together with polling metrics, device configuration, and receiver health signals.

## Two things to know on day one

These two facts are not Netdata-specific. They are how trap-based monitoring works, and they prevent the most common first-week mistakes.

### Traps are push events

Netdata does not auto-create trap listener jobs or configure devices to send traps. You create explicit SNMP trap listener jobs, then configure devices to send Trap or INFORM notifications to the Netdata listener address and port.

Trap reception is event-driven. The collector metrics interval controls how often receiver self-metrics are published, not whether incoming traps are received or sampled.

Because traps are pushed by devices, a quiet trap receiver can mean several different things: the network is quiet, the devices are not configured to send traps, traffic is blocked, credentials do not match, the source is not allowed, or the receiver pipeline has a problem. Confirm first receipt in Logs and watch the receiver metrics before treating silence as normal.

### Trap count is only the start

A trap count tells you that the receiver saw activity. It does not tell you whether the activity is severe.

Use the resolved trap name, category, severity, source, and varbinds to understand meaning. Then check dedup suppression, drops, decode errors, write failures, and OTLP export errors to understand whether the receiver handled the traffic cleanly.

## What ships with the collector

The Netdata SNMP trap listener ships with:

- **Explicit listener jobs** for trap reception. Netdata does not create trap listener jobs automatically.
- **Protocol support** for SNMPv1 Trap, SNMPv2c Trap/INFORM, and SNMPv3 Trap/INFORM with USM authentication and privacy.
- **SNMPv3 engine ID controls** with static allowlists and optional dynamic engine ID discovery for sender/user pairs.
- **Direct journal storage** enabled by default for explicit jobs on Linux, with structured trap log entries under the Netdata log directory. The per-job root defaults to `/var/log/netdata/traps/<job>/`, and `journalctl --directory` reads its machine-id child.
- **Embedded log querying** through the Cloud-required `snmp:traps` logs Function. Direct-journal jobs appear as selectable log sources; OTLP-only jobs do not.
- **Optional OTLP/gRPC export** that sends traps as OTLP LogRecords. If both direct journal and OTLP export are enabled, both outputs receive traps. At least one output backend, direct journal or OTLP, must be enabled for a job to start; direct journal requires Linux, while OTLP-only jobs are not blocked by that Linux journal requirement.
- **Out-of-box trap profiles**: 800+ stock vendor profiles covering 6,000+ MIBs and 150,000+ trap definitions, used to resolve OIDs to trap names, categories, severities, and varbind labels.
- **Per-OID overrides** for category, severity, and labels when local policy differs from profile defaults.
- **A closed category taxonomy**: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, and `unknown`.
- **A closed severity taxonomy**: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, and `debug`.
- **Optional deduplication** that suppresses repeated identical traps inside a configured window and writes summary entries.
- **Self-metrics** for pipeline counters, events by category and severity, errors, dedup suppression, source health, and profile-metric diagnostics.
- **Optional profile-defined trap metrics** with cardinality limits for selected events that should also be represented as Netdata metrics.

The shipped profile pack and local custom profiles assign categories and severities during decoding. See [SNMP trap profiles](/docs/snmp-traps/trap-profiles.md) for how trap definitions, overrides, and custom profiles affect decoded output.

## Where to start

Pick the page that matches your situation:

- **Set up collection** - [Installation](/docs/snmp-traps/installation.md), [Quick Start](/docs/snmp-traps/quick-start.md), and [Configuration](/docs/snmp-traps/configuration.md).
- **Understand decoded traps** - [SNMP trap profiles](/docs/snmp-traps/trap-profiles.md), [Enrichment and identity](/docs/snmp-traps/enrichment.md), [Use SNMP trap data](/docs/snmp-traps/usage-and-output.md), and [SNMP trap field reference](/docs/snmp-traps/field-reference.md).
- **Query, export, and operate** - [Journal and querying](/docs/snmp-traps/journal-and-querying.md), [Forward SNMP traps to SIEM and log systems](/docs/snmp-traps/forwarding-to-siem.md), [SNMP trap metrics and alerts](/docs/snmp-traps/metrics-and-alerts.md), [SNMP trap sizing and capacity planning](/docs/snmp-traps/sizing-and-capacity.md), and [SNMP trap validation and data quality](/docs/snmp-traps/validation-and-data-quality.md).
- **Investigate or fix problems** - [Investigation playbooks](/docs/snmp-traps/investigation-playbooks.md), [SNMP trap anti-patterns](/docs/snmp-traps/anti-patterns.md), and [Troubleshooting SNMP traps](/docs/snmp-traps/troubleshooting.md).
