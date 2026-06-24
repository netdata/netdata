<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/snmp-traps/alerts.md"
sidebar_label: "Alerts"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'alerts', 'health', 'severity', 'routing', 'silencing']
endmeta-->

<!-- markdownlint-disable-file -->

# SNMP Trap Alerts

Use this page when you need to route, silence, or understand the default health alerts that ship for the SNMP trap receiver. The alert thresholds, windows, and template names are listed below so you can tie each alert to the metric it watches.

The default alerts watch receiver health and high-severity trap flow. They do not assert the complete health state of the sending devices. For the underlying receiver metrics these alerts read, see [Metrics](/docs/npm/snmp-traps/metrics.md).

## Default health alerts

Netdata ships default health alerts for SNMP trap receiver metrics. They alert on receiver health and high-severity trap flow; they do not assert the complete health state of the sending devices.

Default alerts check every minute and use 5-minute or 10-minute average windows, depending on the alert. Use the template names below when routing, silencing, or reviewing default alert behavior.

Severity alerts:

- Emergency traps alert critically when any `emerg` traps are received over the 5-minute window.
- Alert and critical traps warn when any `alert` or `crit` traps are received, and become critical above 5 events/s for `alert` severity or above 10 events/s for `crit` severity.
- Error and warning traps alert only at higher rates. The important default storm thresholds are `err` above 10 events/s for warning and 100 events/s for critical, and `warning` above 100 events/s for warning and 1000 events/s for critical.
- `notice`, `info`, and `debug` severities do not alert by default.

Processing and policy alerts:

- Decode failures, unresolved profile templates, malformed PDUs, authentication failures, USM failures, unknown engine IDs, INFORM response failures, binary-encoded fields, profile load failures, OTLP export failures, and listener read failures warn when they appear.
- Allowlist drops and rate-limit drops alert only at higher sustained rates. The important default thresholds are above 10 errors/s for warning and above 100 errors/s for critical.
- Unknown OIDs are exposed as a metric but do not alert by default, because uncovered vendor traps are common in mixed networks.

Output and storage alerts:

- Journal write failures warn when any failures appear and become critical above 1 error/s. Treat these as urgent because direct-journal jobs cannot commit local trap rows while writes are failing.
- OTLP export failures warn when they appear. No critical threshold is shipped by default. For OTLP-only jobs, export failures mean the configured downstream OTLP receiver is not receiving trap rows.

Deduplication alert:

- High deduplication suppression warns above 1000 suppressed events/s and becomes critical above 10000 suppressed events/s. This means repeated traps are being intentionally collapsed at storm-level rate.

## Default alert templates {#default-alert-templates}

| Template | Context / dimension | Default threshold |
|---|---|---|
| `snmp_trap_emergency_events` | `snmp.trap.severity` / `emerg` | critical above 0 events/s (5m avg) |
| `snmp_trap_alert_events` | `snmp.trap.severity` / `alert` | warning above 0, critical above 5 events/s (5m avg) |
| `snmp_trap_critical_events` | `snmp.trap.severity` / `crit` | warning above 0, critical above 10 events/s (5m avg) |
| `snmp_trap_error_events` | `snmp.trap.severity` / `err` | warning above 10, critical above 100 events/s (5m avg) |
| `snmp_trap_warning_event_storm` | `snmp.trap.severity` / `warning` | warning above 100, critical above 1000 events/s (10m avg) |
| `snmp_trap_decode_errors` | `snmp.trap.errors` / `decode_failed` | warning above 0 errors/s (10m avg) |
| `snmp_trap_template_unresolved` | `snmp.trap.errors` / `template_unresolved` | warning above 0 errors/s (10m avg) |
| `snmp_trap_malformed_pdus` | `snmp.trap.errors` / `malformed_pdu` | warning above 0 errors/s (10m avg) |
| `snmp_trap_allowlist_drops` | `snmp.trap.errors` / `dropped_allowlist` | warning above 10, critical above 100 errors/s (10m avg) |
| `snmp_trap_rate_limited` | `snmp.trap.errors` / `rate_limited` | warning above 10, critical above 100 errors/s (10m avg) |
| `snmp_trap_auth_failures` | `snmp.trap.errors` / `auth_failures` | warning above 0 errors/s (10m avg) |
| `snmp_trap_usm_failures` | `snmp.trap.errors` / `usm_failures` | warning above 0 errors/s (10m avg) |
| `snmp_trap_unknown_engine_id` | `snmp.trap.errors` / `unknown_engine_id` | warning above 0 errors/s (10m avg) |
| `snmp_trap_inform_response_failures` | `snmp.trap.errors` / `inform_response_failed` | warning above 0 errors/s (10m avg) |
| `snmp_trap_binary_encoded_fields` | `snmp.trap.errors` / `binary_encoded` | warning above 0 errors/s (10m avg) |
| `snmp_trap_profile_load_failures` | `snmp.trap.errors` / `profile_load_failed` | warning above 0 errors/s (10m avg) |
| `snmp_trap_journal_write_failures` | `snmp.trap.errors` / `journal_write_failed` | warning above 0, critical above 1 errors/s (5m avg) |
| `snmp_trap_otlp_export_failures` | `snmp.trap.errors` / `otlp_export_failed` | warning above 0 errors/s (10m avg) |
| `snmp_trap_listener_read_failures` | `snmp.trap.errors` / `listener_read_failed` | warning above 0 errors/s (10m avg) |
| `snmp_trap_high_dedup_suppression` | `snmp.trap.dedup_suppressed` / `suppressed` | warning above 1000, critical above 10000 events/s (10m avg) |

## Kernel UDP buffer drops

The trap receiver also benefits from the system-level `1m_ipv4_udp_receive_buffer_errors` alert on `ipv4.udperrors`. It catches datagrams the kernel drops before they ever reach the collector, which the receiver's own pipeline metrics cannot see. That alert is routed `to: silent` by default — route it to a recipient to be notified of kernel UDP drops during a storm. See [Sizing and Capacity](/docs/npm/snmp-traps/sizing-and-capacity.md#kernel-udp-buffer-drops).

## Routing and silencing

These are standard Netdata health alerts. Route or silence them like any other alert — by recipient, role, or template name — using your health notification configuration. The template names in the table above are the handles you use to target a specific alert when you adjust its routing, raise or lower a threshold, or silence it for a known steady-state source.

## What's next

- [Metrics](/docs/npm/snmp-traps/metrics.md) - Read the receiver pipeline, processing errors, dedup counters, and source metrics these alerts watch.
- [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md) - Investigate the failures that raise these alerts.
- [Configuration](/docs/npm/snmp-traps/configuration.md) - Tune allowlists, rate limits, deduplication, and output backends that change alert behavior.
