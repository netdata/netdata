<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/metrics-and-alerts.md"
sidebar_label: "Metrics and Alerts"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'metrics', 'alerts', 'health', 'deduplication', 'receiver pipeline']
endmeta-->

<!-- markdownlint-disable-file -->

# Metrics and Alerts

Use this page to check whether an SNMP trap receiver is healthy, whether traps are flowing, what the receiver is dropping or suppressing, and what the default health alerts mean.

SNMP trap metrics describe the receiver and the traps that reached it. They do not prove the complete state of a device. Absence of traps is not proof of absence of events, because a device may be quiet, misconfigured, blocked by the network, or sending to another destination.

For setup and related operator workflows, see:

- [Configuration](/docs/snmp-traps/configuration.md)
- [Usage and Output](/docs/snmp-traps/usage-and-output.md)
- [Validation and Data Quality](/docs/snmp-traps/validation-and-data-quality.md)
- [Troubleshooting](/docs/snmp-traps/troubleshooting.md)

## Receiver pipeline

The **SNMP trap receiver pipeline** chart shows per-job event rates for the receiver path. Use it first when deciding whether the listener is receiving packets and whether accepted traps are reaching the configured output backend.

| Dimension | What it means | Operator use |
|---|---|---|
| `received` | A UDP packet entered the trap handler. | Confirms traffic reached the listener process. |
| `decoded` | The packet parsed as an SNMP trap or INFORM PDU. | Compare with `received` to find packets rejected before or during decode, such as source allowlist drops, unsupported sniffed versions, authentication failures, or malformed packets. |
| `accepted` | The trap became a normal trap entry after source checks, profile lookup, template rendering, and source attribution. | Compare with `decoded` to find post-decode policy or source-identity checks that stopped a decoded trap before acceptance. |
| `committed` | The trap was accepted by the authoritative output writer. | This is the strongest receiver-side signal that the receiver accepted the trap for local storage or export. Backend failures are exposed by write and export error metrics. |
| `dedup_suppressed` | A repeated trap matched deduplication and was intentionally summarized instead of written as another row. | Expected during repeated events when deduplication is enabled. |
| `dropped` | The receiver ended packet handling without a committed normal trap row, dedup-suppression result, or authoritative write-failed outcome. Decode-error rows can also increment this dimension in parallel with their error dimension. | Compare with processing errors and decode-error rows to separate decode failures, policy drops, rate limits, and other unfinished packet paths. |
| `write_failed` | The authoritative output writer rejected the trap. | In journal-only and journal+OTLP jobs, journal write failures increment `write_failed` with the error dimension `journal_write_failed`. In OTLP-only jobs, synchronous queue-full failures and asynchronous export failures are terminal and increment `write_failed` with the error dimension `otlp_export_failed`. In journal+OTLP jobs, secondary OTLP enqueue or export failures raise `otlp_export_failed` without raising `write_failed`. |

Interpret gaps in order:

- `received` rising while `decoded` is flat usually points to source allowlist, unsupported sniffed version, authentication, or malformed packet problems before a parsed trap exists.
- `decoded` rising while `accepted` is flat usually points to post-decode policy checks: version or community allowlist drops, engine ID checks, or rate limits. Profile lookup problems and template issues are data-quality signals on traps that can still become accepted entries. Dedup suppression does not keep `accepted` flat; it is tracked separately.
- `received` rising while `committed` is flat means traps are reaching the receiver but are not being accepted by the configured output writer. Check `dropped`, `dedup_suppressed`, `write_failed`, and the error dimensions.
- `write_failed` rising points to the authoritative write path for the job: local direct journal when `journal.enabled` is `true`, or OTLP when journal is disabled. In OTLP-only jobs, both synchronous queue-full failures and asynchronous export failures increment `write_failed` with `otlp_export_failed`; when journal and OTLP are both enabled, check `otlp_export_failed` separately for secondary OTLP enqueue or export failures that do not raise `write_failed`.
- `dedup_suppressed` rising means repeated rows were intentionally collapsed. Confirm the deduplication policy before treating this as loss.

## Categories and severities

Category and severity charts count traps after receiver commit. They classify trap rows accepted by the configured writer; they are not polling metrics and do not prove current device state.

The **SNMP trap events** chart uses this fixed category set:

| Category | Typical meaning |
|---|---|
| `state_change` | A device, interface, service, sensor, peer, or component changed state. |
| `config_change` | Configuration changed on the sender. |
| `security` | Security-related event. |
| `auth` | Authentication or authorization event. |
| `license` | License state or entitlement event. |
| `mobility` | Wireless or roaming event. |
| `diagnostic` | Diagnostic, test, or health-reporting event. |
| `unknown` | Netdata could not classify the trap with a loaded profile or override. |

The **SNMP trap events by severity** chart uses this fixed severity set:

| Severity | Operator meaning |
|---|---|
| `emerg` | System unusable. Treat as immediate device-side emergency. |
| `alert` | Immediate action required. |
| `crit` | Critical condition. |
| `err` | Error condition. |
| `warning` | Warning condition. |
| `notice` | Normal but significant condition. |
| `info` | Informational event. |
| `debug` | Debug-level event. |

Severity and category come from trap profiles or local overrides. If `unknown` grows, check profile coverage and any custom profile load errors.

## Processing errors

The **SNMP trap processing errors** chart shows per-job error rates. Some errors can still produce a `decode_error` row if the receiver can write one; other drops happen before a row exists and are visible only as metrics.

| Dimension | What to check |
|---|---|
| `unknown_oid` | The trap was decoded, but no loaded profile matched its trap OID. Add or fix profile coverage if the trap should be classified. |
| `decode_failed` | The packet could not be decoded and did not match a more specific decode error class. Check sender version and packet validity. |
| `template_unresolved` | A profile template referenced a missing varbind or field. Check custom profile templates and the trap payload. |
| `malformed_pdu` | The PDU was structurally invalid, too large, missing required trap fields, or otherwise malformed. Check sender firmware and packet capture. |
| `dropped_allowlist` | The sender, SNMP version, or community was outside the allowed configuration. Check `allowlist.source_cidrs`, `versions`, and `communities`. |
| `rate_limited` | A sender exceeded the configured per-source rate limit. Check `rate_limit` settings and sender storm behavior. |
| `auth_failures` | Authentication or decryption failed. Check SNMP community or SNMPv3 auth/privacy settings. |
| `usm_failures` | SNMPv3 USM processing failed. Check users, protocols, keys, and sender engine state. |
| `unknown_engine_id` | SNMPv3 engine ID was not accepted or could not be resolved according to the job configuration. When `dynamic_engine_id_discovery` is enabled, first-time accepted `(engineID, username)` registrations also increment this counter once per job lifetime as a visibility signal. Check engine ID whitelist, dynamic discovery settings, cap exhaustion, invalid sender state, or unauthorized senders. |
| `inform_response_failed` | The receiver failed to send an INFORM acknowledgement. Check local socket and network path back to the sender. |
| `binary_encoded` | Structured fields were written with binary journal encoding for CWE-117 log-injection protection. Applies to the direct-journal path; OTLP-only jobs keep this counter at zero. A low background rate can be normal with known binary varbinds, but the default `snmp_trap_binary_encoded_fields` alert warns on any sustained non-zero rate over 10 minutes. Tune or silence that alert for known steady-state binary sources; investigate new or rising rates by checking profile labels, rendered varbind values with control characters, invalid UTF-8, or binary payload values. |
| `profile_load_failed` | Trap profile loading or lookup failed. Check custom profile YAML and profile directories. |
| `journal_write_failed` | Direct journal write failed. Check disk space, permissions, and the per-job journal directory. |
| `otlp_export_failed` | OTLP export failed. Check endpoint, credentials, TLS, queue pressure, and network path. |
| `listener_read_failed` | The listener failed to read from a bound UDP socket. Check operating-system socket errors and listener lifecycle logs. |

For configuration details, see the [rate limiting, deduplication, and output backend sections](/docs/snmp-traps/configuration.md).

## Deduplication suppression

The **SNMP trap dedup suppressed events** chart appears when deduplication is enabled. Its dimension is `suppressed`, and it counts repeated matching traps that were intentionally suppressed during the deduplication window. The receiver pipeline chart tracks the same suppressed traps as `dedup_suppressed`, so both dimensions rise and fall together.

Deduplication protects storage and downstream systems from repeated identical rows. A suppressed trap is not committed as a normal trap row and does not update profile-defined metrics. It is represented by deduplication metrics and summary rows instead, so the suppression remains auditable.

Use this chart with the pipeline:

- `dedup_suppressed` rising with stable `committed` means repeated events are being collapsed as configured.
- `dedup_suppressed` rising at very high rate can indicate a device-side trap storm.
- If distinct resources are being collapsed together, review `dedup.key_varbinds` in [Configuration](/docs/snmp-traps/configuration.md).

## Sources and attribution

Source metrics help answer which senders are active and which senders are affected by drops or backend failures.

| Chart | Dimensions | How to use it |
|---|---|---|
| **SNMP trap active sources** | `active` | Number of source identities currently tracked for the job. |
| **SNMP trap source attribution** | `vnode`, `fallback`, `ambiguous`, `failed`, `overflow_dropped`, `source_transitions` | Shows how the receiver assigned trap activity to source identities and whether attribution was ambiguous, failed, exceeded the source-metric cap, or changed route. |
| **SNMP trap source pipeline** | `accepted`, `committed`, `dedup_suppressed`, `write_failed` | Per-source view of normal trap handling after attribution. |
| **SNMP trap source-attributed errors** | `unknown_oid`, `template_unresolved`, `profile_load_failed`, `journal_write_failed`, `otlp_export_failed` | Per-source view of errors that can be attributed to a trap entry. |
| **SNMP trap source last seen** | `seconds_ago` | Time since the receiver last saw activity for that source identity. |

Per-source charts are labeled by `job_name`, `source_id`, and `source_kind`. When a trap can be tied to a Netdata vnode, the source identity uses that vnode. Otherwise the receiver falls back to the selected trap source, such as the enriched source, entry source, or UDP peer. Fallback source IDs are privacy-preserving hashes by default; use trap rows in [Usage and Output](/docs/snmp-traps/usage-and-output.md) when you need readable source fields.

Source-attributed receiver metrics are bounded to 2000 active sources per job. Inactive sources expire after 60 successful collection cycles. Accepted traps can still be committed if source metric attribution fails or the source cap is full; in that case, per-job pipeline totals can be higher than the sum of per-source charts.

This source-metric cap applies to the built-in source receiver charts: `snmp.trap.source_pipeline`, `snmp.trap.source_errors`, and `snmp.trap.source_last_seen`. Profile-defined metrics have separate cardinality limits; see [Profile-defined metrics](#profile-defined-metrics).

Interpret source charts carefully:

- A high `active` count can be normal on large networks, but sudden growth can also mean unexpected senders or relay behavior.
- `ambiguous` means the receiver saw conflicting or rejected source evidence. Check source attribution fields in trap rows.
- `failed` means the receiver could not create a source metric identity for an entry.
- `overflow_dropped` means source metric tracking hit its cap. Accepted traps can still be committed, but per-source metric visibility is bounded.
- `source_transitions` means the same raw source route changed to a different metric identity. Check relay, vnode, and source attribution configuration.

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

| Template | Context / dimension | Default threshold |
|---|---|---|
| `snmp_trap_emergency_events` | `snmp.trap.severity` / `emerg` | critical above 0 over 5m |
| `snmp_trap_alert_events` | `snmp.trap.severity` / `alert` | warning above 0, critical above 5 over 5m |
| `snmp_trap_critical_events` | `snmp.trap.severity` / `crit` | warning above 0, critical above 10 over 5m |
| `snmp_trap_error_events` | `snmp.trap.severity` / `err` | warning above 10, critical above 100 over 5m |
| `snmp_trap_warning_event_storm` | `snmp.trap.severity` / `warning` | warning above 100, critical above 1000 over 10m |
| `snmp_trap_decode_errors` | `snmp.trap.errors` / `decode_failed` | warning above 0 over 10m |
| `snmp_trap_template_unresolved` | `snmp.trap.errors` / `template_unresolved` | warning above 0 over 10m |
| `snmp_trap_malformed_pdus` | `snmp.trap.errors` / `malformed_pdu` | warning above 0 over 10m |
| `snmp_trap_allowlist_drops` | `snmp.trap.errors` / `dropped_allowlist` | warning above 10, critical above 100 over 10m |
| `snmp_trap_rate_limited` | `snmp.trap.errors` / `rate_limited` | warning above 10, critical above 100 over 10m |
| `snmp_trap_auth_failures` | `snmp.trap.errors` / `auth_failures` | warning above 0 over 10m |
| `snmp_trap_usm_failures` | `snmp.trap.errors` / `usm_failures` | warning above 0 over 10m |
| `snmp_trap_unknown_engine_id` | `snmp.trap.errors` / `unknown_engine_id` | warning above 0 over 10m |
| `snmp_trap_inform_response_failures` | `snmp.trap.errors` / `inform_response_failed` | warning above 0 over 10m |
| `snmp_trap_binary_encoded_fields` | `snmp.trap.errors` / `binary_encoded` | warning above 0 over 10m |
| `snmp_trap_profile_load_failures` | `snmp.trap.errors` / `profile_load_failed` | warning above 0 over 10m |
| `snmp_trap_journal_write_failures` | `snmp.trap.errors` / `journal_write_failed` | warning above 0, critical above 1 over 5m |
| `snmp_trap_otlp_export_failures` | `snmp.trap.errors` / `otlp_export_failed` | warning above 0 over 10m |
| `snmp_trap_listener_read_failures` | `snmp.trap.errors` / `listener_read_failed` | warning above 0 over 10m |
| `snmp_trap_high_dedup_suppression` | `snmp.trap.dedup_suppressed` / `suppressed` | warning above 1000, critical above 10000 over 10m |

## Profile-defined metrics

Profile-defined metrics convert selected committed traps into time-series. They are disabled by default and must be enabled with `profile_metrics` in [Configuration](/docs/snmp-traps/configuration.md).

Important behavior:

- Profile metrics update after the configured writer accepts the trap. For direct-journal jobs, this means the trap was accepted into the direct-journal writer. For OTLP-only jobs, this means the trap was queued for OTLP export. Later asynchronous writer or export failures do not roll back an already updated profile metric.
- Dedup-suppressed traps do not update profile metrics.
- Synchronous write failures, including journal write failures and OTLP queue-full failures, do not update profile metrics.
- Cardinality limits protect the node by bounding enabled rules, sources, resources per source, and total metric instances per job.
- Over-cap profile metric instances are skipped and counted by diagnostics; the accepted trap can still be committed.

Selection modes decide which loaded profile metric rules run:

- `none`: no rules are evaluated.
- `auto`: runs only rules marked safe for automatic use.
- `exact`: runs only rule names listed in `profile_metrics.include`.
- `combined`: runs automatic rules plus rule names listed in `profile_metrics.include`.

When at least one profile metric rule is selected, Netdata also emits the dynamic **SNMP trap profile metric diagnostics** chart, context `snmp.trap.profile_metric_diagnostics`, for the listener job.

| Dimension | What it means | Operator use |
|---|---|---|
| `rule_missed` | A selected rule did not match the trap, or a missing value used `missing: drop`. | Expected when a rule applies only to some traps. Sudden changes can mean the trap payload or profile predicates changed. |
| `extraction_failed` | A selected rule matched but could not extract a required runtime value. | Check the profile rule, varbind type, and trap payload. |
| `attribution_failed` | Netdata could not derive or accept a source identity for the metric instance. | Check source attribution and profile metric identity settings. |
| `overflow_dropped` | A new metric instance exceeded source, resource, chart, or job cardinality caps. | Tighten rule identity, reduce selected rules, or adjust reviewed cardinality limits. |
| `source_transitions` | The same source route changed between fallback and vnode or device attribution. | Check enrichment, vnode matching, relays, and whether sender identity is stable. |

Enable only rules with bounded identity and labels. For trap row fields and deduplication summary rows, see [Usage and Output](/docs/snmp-traps/usage-and-output.md). For validation workflow and data-quality checks, see [Validation and Data Quality](/docs/snmp-traps/validation-and-data-quality.md) and [Troubleshooting](/docs/snmp-traps/troubleshooting.md).
