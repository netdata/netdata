<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/validation-and-data-quality.md"
sidebar_label: "Validation and Data Quality"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'validation', 'data quality', 'source identity', 'trusted relays', 'otlp', 'siem']
endmeta-->

<!-- markdownlint-disable-file -->

# Validation and Data Quality

Use this page before relying on SNMP trap logs during incidents. The goal is to prove that trap collection is correct, secure, and useful: packets arrive, source identity is trustworthy, profiles add useful meaning, storm controls behave as intended, and the selected output backend keeps the events you need.

Validation is not only "did a row appear?" A quiet receiver can be healthy, misconfigured, blocked by network policy, dropping unauthorized packets, or waiting for devices that have not been configured to send traps. Validate trap rows and receiver metrics together.

## Validation checklist

Work through these checks in order during first deployment. Use [Quick Start](/docs/snmp-traps/quick-start.md) to send a known test trap, and use [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) when you need local `journalctl` or Cloud Logs verification commands.

| Check | Evidence to inspect | What good looks like |
|---|---|---|
| First receipt | `TRAP_REPORT_TYPE`, `TRAP_JOB`, `TRAP_OID`, `TRAP_NAME`, `TRAP_SOURCE_IP`, `snmp.trap.pipeline` | A known test trap appears as `TRAP_REPORT_TYPE=trap`, and `received`, `decoded`, `accepted`, and `committed` move for the listener job. |
| Source identity | `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_ENRICHMENT.source` | Direct senders usually have matching source and UDP peer. Relayed traps show why Netdata selected the reported source. OTLP-only jobs use equivalent OTLP attributes instead of journal field names. |
| Credentials and version | `TRAP_VERSION`, `TRAP_REPORT_TYPE=decode_error`, `TRAP_DECODE_ERROR_KIND`, `snmp.trap.errors` | Expected SNMP versions are accepted. Authentication, USM, unknown engine ID, and malformed packet errors stay at expected levels. |
| Allowed sources and communities | `TRAP_SOURCE_UDP_PEER`, `snmp.trap.errors` `dropped_allowlist`, job `allowlist.source_cidrs`, job `communities` | Only intended UDP peers are admitted. Only intended SNMPv1/v2c communities are accepted once the packet is parsed. |
| Trusted relays | `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_ENRICHMENT.source.method`, `TRAP_ENRICHMENT.source.snmp_trap_address`, `TRAP_ENRICHMENT.source.trusted_relay` | Direct senders use `udp_peer`. Only configured relay peers can make `snmpTrapAddress.0` become the selected source. |
| Profile coverage | `TRAP_NAME`, `MESSAGE`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `snmp.trap.errors` `unknown_oid` | Common device traps resolve to names, rendered messages, categories, severities, and useful varbind labels. Unknown OIDs are tracked as coverage gaps. |
| Profile load health | `snmp.trap.errors` `profile_load_failed`, `template_unresolved` | Loaded profile YAML files parse cleanly, and profile-rendered templates resolve the varbinds and fields they reference. |
| Severity and category policy | `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_TAG_*`, overrides | The resulting urgency and category match local operations policy for the trap OID. |
| Enrichment quality | `TRAP_ENRICHMENT`, `_HOSTNAME`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS`, `TRAP_REVERSE_DNS` | Enrichment fields appear when local context exists, and the audit explains skipped, missing, or ambiguous lookups. |
| Varbind quality | `TRAP_VAR_*`, `TRAP_JSON`, `snmp.trap.errors` `template_unresolved`, `binary_encoded` | Important event payload is queryable through indexed fields, with structured payload and audit details available in `TRAP_JSON`; binary-encoded journal fields are counted and need review. |
| Deduplication | `TRAP_REPORT_TYPE=deduplication_summary`, `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `snmp.trap.dedup_suppressed` | Repeated traps are summarized only when deduplication is intentionally enabled and the fingerprint matches operational expectations. |
| Rate limiting | `snmp.trap.errors` `rate_limited`, job `rate_limit.mode` | Over-limit traffic is counted by `rate_limited`. In `drop` mode the packet is discarded; in `sample` mode it is counted and still enters the pipeline. |
| Journal health | `snmp.trap.pipeline` `committed`, `write_failed`, `snmp.trap.errors` `journal_write_failed`, direct journal path | Healthy direct-journal jobs commit rows and do not report write failures. Direct journal output is Linux-only. |
| OTLP health | `snmp.trap.errors` `otlp_export_failed`, downstream receiver records, OTLP resource attributes | OTLP export succeeds, records have `service.name=netdata-snmptrap` and the expected `service.instance.id`, and downstream records contain the expected trap attributes. |
| Retention | Job `retention` settings and direct journal files | Local retention keeps enough trap history for investigation without unexpectedly evicting needed rows. |
| Downstream SIEM fields | Ingested field names or OTLP attributes | SIEM rules use the fields that actually arrive, especially report type, source, OID/name, category, severity, and payload fields. |

## Start with report type

Always inspect `TRAP_REPORT_TYPE` first:

- `trap` means Netdata decoded and accepted an SNMP Trap or INFORM and wrote it to the configured output.
- `decode_error` means a packet reached the decode-error path but did not become a normal trap row. Inspect `TRAP_DECODE_ERROR_KIND`, `TRAP_DECODE_ERROR`, `TRAP_VERSION`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_SOURCE_UDP_PORT`, `TRAP_PACKET_SIZE`, `TRAP_PACKET_SHA256`, `TRAP_LISTENER`, `TRAP_ENGINE_ID`, and `TRAP_JSON`.
- `deduplication_summary` means repeated matching traps were suppressed during a deduplication period. Inspect `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and `TRAP_JSON`.

Drops that happen before a row is written may be visible only in receiver metrics. Use `snmp.trap.pipeline`, `snmp.trap.errors`, `snmp.trap.dedup_suppressed`, `snmp.trap.sources`, `snmp.trap.source_last_seen`, `snmp.trap.source_attribution`, `snmp.trap.source_pipeline`, and `snmp.trap.source_errors` together with the log rows.

## Validate source identity

Source identity controls attribution, filtering, enrichment, and incident routing. Validate it before using trap data for paging or audit workflows.

Compare `TRAP_SOURCE_IP` with `TRAP_SOURCE_UDP_PEER` (equal for direct senders, different for a trusted-relay override), then inspect the `source` object inside `TRAP_ENRICHMENT` to confirm `selected`, `method`, `trusted_relay`, and any `rejected_candidates`. For the full source-identity model and the step-by-step trust checklist, see [Enrichment](/docs/snmp-traps/enrichment.md#how-to-validate-source-identity). Keep `source.trusted_relays` narrow: a broad range lets senders on that path influence attribution through `snmpTrapAddress.0`.

Treat `TRAP_REVERSE_DNS` as a label only; keep trust decisions based on `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, and `TRAP_ENRICHMENT.source`.

For OTLP-only jobs, validate the same source relationship with OTLP attributes: `snmp.source.ip` is the selected source and `network.peer.address` is the UDP peer. Use the [Field Reference](/docs/snmp-traps/field-reference.md#otlp-mapping-notes) for the journal-to-OTLP mapping.

## Validate credentials and allowed versions

The listener should accept only the SNMP versions and credentials that devices are expected to use.

- For SNMPv1 and SNMPv2c, keep community values out of logs, docs, tickets, and command examples. Use secret references in configuration.
- For SNMPv3, protect authentication and privacy keys, and validate engine ID handling with `TRAP_VERSION`, `TRAP_ENGINE_ID`, and error dimensions such as `auth_failures`, `usm_failures`, and `unknown_engine_id`.
- Decode errors with `TRAP_DECODE_ERROR_KIND=auth_failures`, `usm_failures`, or `unknown_engine_id` usually point to sender or listener credential mismatch, engine ID policy, or unauthorized senders.
- A malformed PDU is not the same problem as authentication failure. Separate `malformed_pdu`, `decode_failed`, `auth_failures`, `usm_failures`, and `unknown_engine_id` when triaging.

## Validate profile coverage

Profile coverage determines whether rows are useful without manual OID lookup.

For normal trap rows, inspect:

- `TRAP_OID`: the numeric trap identifier.
- `TRAP_NAME`: the resolved profile or MIB-qualified name, when known.
- `MESSAGE`: the rendered trap message shown in log views.
- `TRAP_CATEGORY`: the operational category.
- `TRAP_SEVERITY`: the operational severity.
- `TRAP_VAR_*`: indexed event varbinds for filtering and rule logic.
- `TRAP_JSON`: structured payload and audit details.

An unknown OID is a coverage signal, not an ingestion failure. Netdata can still accept and store the trap with `TRAP_OID` and `TRAP_CATEGORY=unknown`. Use `snmp.trap.errors` `unknown_oid`, missing `TRAP_NAME`, and repeated important raw OIDs to decide whether to add or adjust trap profiles or local overrides.

Profile load and template health are separate from OID coverage. Use `snmp.trap.errors` `profile_load_failed` for profile files that cannot be loaded, and `template_unresolved` for rendered text that references missing varbinds or fields.

Validate severity and category as policy, not only parsing. A trap can be decoded correctly while still needing a local override because your organization treats that OID differently. Keep category and severity inside the documented closed values so queries, charts, and downstream rules remain stable.

## Validate enrichment quality

Enrichment should help operators understand the row faster. It should not hide the original evidence.

Use concrete fields for normal filtering:

- `_HOSTNAME`
- `TRAP_DEVICE_VENDOR`
- `TRAP_INTERFACE`
- `TRAP_NEIGHBORS`
- `TRAP_REVERSE_DNS`
- `ND_NIDL_NODE`

These fields are conditional. They appear only when Netdata has enough local context to populate them, such as a matched device, interface, neighbor, reverse-DNS result, or topology node. If a field is absent, use `TRAP_ENRICHMENT.registry`, `TRAP_ENRICHMENT.topology`, `TRAP_ENRICHMENT.interface`, and `TRAP_ENRICHMENT.neighbors` to see whether the lookup was missing, ambiguous, skipped, pending, or in `conflict`.

Use `TRAP_ENRICHMENT` when a field is missing or surprising. It records source selection, local device lookup, interface lookup, neighbor lookup, reverse-DNS state, and applied fields. Common useful statuses include `matched`, `no_match`, `ambiguous`, `skipped`, `pending`, and `conflict`.

Treat enrichment as context. The selected source fields and the original trap OID remain the primary evidence for what Netdata received.

## Validate storm controls

Rate limiting and deduplication change what you see in logs.

- During first validation, keep storm controls disabled or use settings that make their behavior visible.
- If rate limiting is enabled, watch `snmp.trap.errors` `rate_limited`. With `rate_limit.mode: drop`, over-limit packets are discarded; with `rate_limit.mode: sample`, they are counted and still continue through the receiver.
- If deduplication is enabled, expect the first matching trap row plus later `TRAP_REPORT_TYPE=deduplication_summary` rows. Check `dedup.window_sec`, `dedup.cache_max_entries`, and `dedup.key_varbinds` in [Configuration](/docs/snmp-traps/configuration.md) when summaries do not match expectations.
- Read `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and `TRAP_JSON` to understand what was suppressed.
- If one trap OID represents different resources, validate the dedup fingerprint before relying on summaries.

Dedup-suppressed traps are counted in periodic summary entries instead of stored as individual rows. If an operator expects every repeated PDU to be visible, deduplication is the first setting to check.

## Validate backend health and retention

For direct-journal jobs:

- Confirm trap rows appear in the local job source exposed through the `snmp:traps` Function.
- Use [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) for `snmp:traps` Function behavior, Cloud requirements, and local `journalctl --directory` commands.
- Confirm `snmp.trap.pipeline` `committed` moves with accepted traps.
- Watch `snmp.trap.pipeline` `write_failed` and `snmp.trap.errors` `journal_write_failed`.
- Check that retention settings keep enough local history for incident investigation. See [Configuration](/docs/snmp-traps/configuration.md#direct-journal-retention) for retention defaults and settings.

For OTLP export:

- Confirm `snmp.trap.errors` `otlp_export_failed` stays clear.
- Confirm the downstream OTLP receiver stores the expected records.
- Confirm records carry `service.name=netdata-snmptrap` and `service.instance.id=<job>`. See [Forwarding to SIEM](/docs/snmp-traps/forwarding-to-siem.md) for the OTLP export checks.
- Confirm SIEM rules use the downstream field names that actually arrive.

If both direct journal and OTLP are enabled, validate both paths. If a job is OTLP-only, it will not create local journal files and will not appear as a local job source in the `snmp:traps` Function.

## Validate downstream SIEM fields

Before building SIEM rules, decide whether the SIEM receives direct-journal fields or OTLP attributes. The names differ.

For direct-journal ingestion, validate that the SIEM preserves at least:

- `TRAP_REPORT_TYPE`
- `TRAP_JOB`
- `TRAP_SOURCE_IP`
- `TRAP_SOURCE_UDP_PEER`
- `TRAP_OID`
- `TRAP_NAME`
- `TRAP_CATEGORY`
- `TRAP_SEVERITY`
- `TRAP_VERSION`
- `TRAP_VAR_*`
- `TRAP_JSON`
- Decode-error fields when `TRAP_REPORT_TYPE=decode_error`
- Dedup summary fields when `TRAP_REPORT_TYPE=deduplication_summary`

For OTLP ingestion, use the OTLP mapping in [Field Reference](/docs/snmp-traps/field-reference.md). Build rules against report type, selected source, UDP peer, trap OID, trap name, category, severity, version, varbind payload, decode-error details, and dedup summary attributes.

Avoid making `TRAP_JSON` or `TRAP_ENRICHMENT` the primary SIEM grouping key. They are audit payloads and can be large or high-cardinality. Prefer specific stable fields, then inspect the JSON payload when the indexed fields do not answer the question.

## Data quality vs silence

Silence has several meanings:

- No relevant device event happened.
- Devices are not configured to send traps or informs.
- Devices are sending to the wrong address or port.
- Network ACLs or firewalls block UDP delivery.
- Listener `allowlist.source_cidrs` rejects the UDP peer.
- SNMP version, community, USM user, key, or engine ID settings do not match.
- Profile files fail to load, or templates reference fields that are not present in received traps.
- Rate limiting or deduplication changes what appears as rows by design.
- Journal writing or OTLP export is failing.
- Retention has already removed older direct-journal rows.

Do not treat a quiet receiver as proof of device health. Send a known test trap, check `snmp.trap.pipeline`, inspect `snmp.trap.errors`, and verify the configured output backend.

## Security and data handling cautions

Trap data can include sensitive network and device context. Validate useful data, but minimize what you copy, forward, or expose. Treat communities, SNMPv3 keys, and OTLP headers as credentials (use secret references), and review `TRAP_JSON`, `TRAP_VAR_*`, and `TRAP_ENRICHMENT` before forwarding because device-provided varbinds and enrichment can carry usernames, interface descriptions, MACs, asset tags, locations, or public addresses. For the complete per-field sensitivity guidance, see [Field Reference](/docs/snmp-traps/field-reference.md#sensitive-data-cautions).

Use RFC 5737 example IPs such as `192.0.2.10`, `198.51.100.20`, and `203.0.113.5` in examples. Use placeholders for secrets, device names, organization names, and private endpoints.

## Related pages

- [Enrichment](/docs/snmp-traps/enrichment.md) - Source identity, trusted relays, reverse DNS, and enrichment audit details.
- [Configuration](/docs/snmp-traps/configuration.md) - Source controls, credentials, deduplication, rate limiting, retention, and output settings.
- [Trap Profiles](/docs/snmp-traps/trap-profiles.md) - How profiles turn OIDs and varbinds into names, categories, severities, labels, and profile-defined metrics.
- [Field Reference](/docs/snmp-traps/field-reference.md) - Complete field meanings, population rules, sensitive-data cautions, and OTLP mapping.
- [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) - Local `journalctl` and Cloud Logs query workflows.
- [Forwarding to SIEM](/docs/snmp-traps/forwarding-to-siem.md) - OTLP export, resource attributes, and SIEM field validation.
- [Metrics](/docs/snmp-traps/metrics.md) - Receiver metrics, pipeline counters, and errors.
- [Alerts](/docs/snmp-traps/alerts.md) - Default health alert behavior on these receiver metrics.
- [Troubleshooting](/docs/snmp-traps/troubleshooting.md) - Failure investigation paths when validation does not match expectations.
- [Anti-Patterns](/docs/snmp-traps/anti-patterns.md) - Configurations and operational habits to avoid.
