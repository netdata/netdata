<!-- markdownlint-disable-file -->

# Investigation Playbooks

Use these playbooks during SNMP trap incidents when you need a fast, repeatable investigation path. They are written for NOC, NetOps, SecOps, SRE, and incident response teams.

Start from the operator signal, then check the trap rows and receiver metrics together. Trap absence is not proof that nothing happened: a device may be quiet, misconfigured, blocked by the network, rejected by policy, rate limited, deduplicated, or failing to write or export rows.

## Before you start

Use these rules for every playbook:

- Check `TRAP_REPORT_TYPE` first. It tells you whether the row is a normal trap, a decode error, or a deduplication summary.
- Treat `TRAP_SEVERITY` and `TRAP_CATEGORY` as profile policy. Use them to sort urgency, then confirm the meaning with `TRAP_NAME`, `TRAP_OID`, `TRAP_VAR_*`, `TRAP_JSON`, device context, and operational evidence.
- Use receiver metrics when rows are missing or incomplete. Compare `received`, `decoded`, `accepted`, `committed`, `dedup_suppressed`, `dropped`, and `write_failed` in `snmp.trap.pipeline`, then inspect `snmp.trap.errors` and source metrics.
- For OTLP-only jobs, inspect the equivalent downstream log attributes instead of local journal field names. Use the OTLP mapping in [Field Reference](/docs/npm/snmp-traps/field-reference.md#otlp-mapping-notes).
- Keep tickets clean. Do not paste raw sensitive payloads, real SNMP communities, SNMPv3 secrets, full `TRAP_JSON`, full `TRAP_ENRICHMENT`, organization identifiers, or public IPs that identify your environment into tickets. Use placeholders and RFC 5737 example IPs such as `192.0.2.10`, `198.51.100.20`, and `203.0.113.5`.

For field meanings, query examples, metrics, validation checks, and deeper troubleshooting, see:

- [Usage and Output](/docs/npm/snmp-traps/usage-and-output.md)
- [Field Reference](/docs/npm/snmp-traps/field-reference.md)
- [Journal and Querying](/docs/npm/snmp-traps/journal-and-querying.md)
- [Metrics](/docs/npm/snmp-traps/metrics.md)
- [Alerts](/docs/npm/snmp-traps/alerts.md)
- [Validation and Data Quality](/docs/npm/snmp-traps/validation-and-data-quality.md)
- [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md)

## Trap storm or noisy source

**Situation.** One source is flooding the receiver with traps, and the noise is drowning out the events that matter.

- **First signal:** High trap row volume from one source, a high-rate severity alert, rising `snmp.trap.errors` `rate_limited`, rising `dedup_suppressed`, or rising `dropped`.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_VAR_*`, `TRAP_JSON`, `snmp.trap.pipeline`, `snmp.trap.errors`, `snmp.trap.dedup_suppressed`, `snmp.trap.source_pipeline`, `snmp.trap.source_errors`, and `snmp.trap.source_last_seen`.
- **Likely interpretation:** One device or relay is sending repeated events, the same event is being deduplicated, a source exceeded the rate limit, or the receiver is dropping traffic before it becomes committed rows.
- **Next action:** Group by `TRAP_SOURCE_IP`, then by `TRAP_NAME` or `TRAP_OID`. Confirm whether `dedup_suppressed` or `rate_limited` explains missing repeated rows, then work with the device owner to stop the cause or adjust receiver policy only after confirming the storm is expected.

## Critical, security, or authentication traps

**Situation.** A high-severity or security trap just fired, and you need to confirm what the device reported before escalating.

- **First signal:** A default severity alert for `emerg`, `alert`, or `crit`, or a sudden increase in `TRAP_CATEGORY=security` or `TRAP_CATEGORY=auth`.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_VAR_*`, `TRAP_JSON`, `TRAP_ENRICHMENT`, receiver pipeline metrics, receiver error metrics, and source metrics.
- **Likely interpretation:** The sender reported an urgent condition, security event, authentication event, or policy-relevant device event. Severity and category indicate profile or override policy, not final proof of device state.
- **Next action:** Open the matching trap rows, identify the source and trap identity, and verify the event with device-side or security-system evidence. Escalate using the confirmed source, trap name or OID, category, severity, and a minimized payload summary.

## Link or interface state changes

**Situation.** A link is reporting up/down transitions and you need to tell a one-time change from a flapping interface.

- **First signal:** `TRAP_CATEGORY=state_change`, repeated state-change traps from one source, or known interface traps such as `TRAP_NAME=IF-MIB::linkDown` or `TRAP_NAME=IF-MIB::linkUp`.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, relevant `TRAP_VAR_*` interface fields, `TRAP_JSON`, `TRAP_ENRICHMENT`, `snmp.trap.pipeline`, and source metrics.
- **Likely interpretation:** A device is reporting an interface or link state transition. Repeated alternating traps can indicate a flap; a single row can be a one-time administrative or physical event.
- **Next action:** Filter the same source and time window for matching up/down trap names or OIDs. Use `TRAP_VAR_*` fields for the interface identifier when present, and open `TRAP_JSON` only if the indexed varbinds do not identify the affected resource.

## Device restart signals

**Situation.** A device reported a restart, and you need to know whether it explains other missing or unexpected traps around the same time.

- **First signal:** A restart trap such as `TRAP_NAME=SNMPv2-MIB::coldStart` or `TRAP_NAME=SNMPv2-MIB::warmStart`, or a cluster of restart-like trap OIDs from the same source.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_VAR_*`, `TRAP_JSON`, `snmp.trap.pipeline`, and `snmp.trap.source_last_seen`.
- **Likely interpretation:** The sender is reporting a device or agent restart. A restart signal can also explain later missing traps if the device was rebooting, reloading, or changing SNMP sender state.
- **Next action:** Build a short timeline around the restart row. Check whether traps stopped before the restart or resumed after it, then confirm with device uptime, device logs, or change records without copying sensitive payload fields.

## Missing expected traps

**Situation.** A trap you expected never showed up, and you need to find where in the path it was lost — or whether it was ever sent.

- **First signal:** An expected trap did not appear in Netdata Logs, a downstream system has no matching event, or a source has not been seen recently.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, decode-error fields, dedup summary fields, `snmp.trap.pipeline`, `snmp.trap.errors`, `snmp.trap.dedup_suppressed`, `snmp.trap.source_pipeline`, `snmp.trap.source_errors`, and `snmp.trap.source_last_seen`.
- **Likely interpretation:** The trap may not have been sent, may have been blocked before the listener, may have been rejected by allowlist or credentials, may have failed decode, may have been rate limited or deduplicated, or may have failed to write or export.
- **Next action:** Check the receiver pipeline in order: `received`, `decoded`, `accepted`, `committed`. If `received` is flat, check `ipv4.udperrors` (`RcvbufErrors`) for kernel-buffer drops first, then sender and network delivery. If `received` rises but `committed` is flat, inspect `dropped`, `dedup_suppressed`, `write_failed`, and the matching error dimensions.
- **Trap–poll cross-check:** For a critical device, compare the silent trap path against polling. If Netdata's polling collectors still see the device but no traps arrive, the trap path is broken (wrong destination, blocked UDP, allowlist or credential mismatch, or a failing receiver), not the device. Silence is never proof of health — this is why every critical device should be paired with polling.

## Unknown OID or profile coverage issue

**Situation.** Traps are arriving as raw numeric OIDs with no name or category, and you need to decide whether to extend profile coverage.

- **First signal:** Rows with missing `TRAP_NAME`, `TRAP_CATEGORY=unknown`, repeated raw `TRAP_OID` values, or rising `unknown_oid`.
- **Inspect:** `TRAP_OID`, `TRAP_NAME`, `TRAP_CATEGORY`, and `snmp.trap.errors` `unknown_oid` first; for the full field and evidence list, see [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md#unknown-oids-profile-load-failures-or-template-issues).
- **Likely interpretation:** The receiver accepted and stored the trap, but the loaded profiles or overrides did not classify that OID. This is a coverage issue, not necessarily an ingestion failure.
- **Next action:** Collect the numeric `TRAP_OID`, the source, a minimized `TRAP_JSON` summary, and the operational meaning from vendor documentation or device evidence. Add or adjust [Trap Profiles](/docs/npm/snmp-traps/trap-profiles.md) only after confirming the OID and expected severity/category policy.

## SNMPv3 authentication, USM, or engine-ID failures

**Situation.** SNMPv3 traps from a device are being rejected, and you need to tell a credential mismatch from an engine-ID problem.

- **First signal:** Decode-error rows with SNMPv3-related failure classes, or alerts for `auth_failures`, `usm_failures`, or `unknown_engine_id`.
- **Inspect:** `TRAP_REPORT_TYPE=decode_error`, `TRAP_DECODE_ERROR_KIND`, `TRAP_ENGINE_ID`, and the `auth_failures`/`usm_failures`/`unknown_engine_id` error dimensions first; for the full decode-error field list and the v3-silent-but-v2c-works cue, see [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md#snmpv3-auth-privacy-usm-or-engine-id-mismatch).
- **Likely interpretation:** The sender and listener disagree on SNMPv3 user, auth or privacy settings, accepted engine ID policy, or sender state. Repeated `TRAP_PACKET_SHA256` values can indicate the same bad packet pattern repeating.
- **Next action:** Verify the sender identity with `TRAP_SOURCE_IP` and `TRAP_SOURCE_UDP_PEER`, then compare the device and listener SNMPv3 configuration without copying secrets into tickets. Treat `TRAP_ENGINE_ID` as inventory data and share only when necessary.

## Deduplication summaries

**Situation.** Repeated traps are showing up as summary rows instead of individual events, and you need to confirm deduplication is behaving as intended.

- **First signal:** `TRAP_REPORT_TYPE=deduplication_summary`, rising `snmp.trap.dedup_suppressed`, or `dedup_suppressed` rising in the receiver or source pipeline.
- **Inspect:** `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, and `TRAP_REPORT_PERIOD_SEC` first; for the full evidence list and config checks, see [Troubleshooting](/docs/npm/snmp-traps/troubleshooting.md#rate-limiting-or-deduplication-surprise).
- **Likely interpretation:** Repeated matching traps were intentionally summarized instead of written as individual normal trap rows. The summary is job-level reporting, not a normal trap row from one device.
- **Next action:** Use `TRAP_SUPPRESSED_COUNT` and `TRAP_SUPPRESSED_FINGERPRINTS` to separate one repeated event from many repeated event classes. Inspect the first normal trap row and the summary `TRAP_JSON`, then confirm the deduplication policy matches operator expectations.

## Forwarding and SIEM verification

**Situation.** Netdata shows the trap rows, but the downstream SIEM is missing events or fields, and you need to find where the forwarding path diverges.

- **First signal:** Netdata shows trap rows, but the downstream system has no event, incomplete fields, wrong grouping, or unexpected payload size.
- **Inspect:** `TRAP_REPORT_TYPE`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_VAR_*`, `TRAP_JSON`, `TRAP_ENRICHMENT`, decode-error fields, dedup summary fields, `snmp.trap.pipeline` `committed`, `write_failed`, `snmp.trap.errors` `journal_write_failed`, `otlp_export_failed`, and source metrics.
- **Likely interpretation:** The receiver committed the row but the forwarding path, downstream parser, field mapping, or retention policy changed what operators can search. Full payload fields can also be minimized, renamed, or excluded by downstream policy.
- **Next action:** Compare one known trap row in Netdata with the downstream record. Verify that the downstream system preserves report type, source, trap identity, category, severity, selected varbind fields, decode-error fields, and dedup summary fields that your rules depend on.

## What to record in an incident ticket

Record enough context for another operator to reproduce the investigation without exposing sensitive data:

- Time window and affected listener.
- `TRAP_JOB`.
- `TRAP_REPORT_TYPE`.
- Redacted `TRAP_SOURCE_IP` and `TRAP_SOURCE_UDP_PEER`, or RFC 5737 examples in public material.
- `TRAP_NAME` and `TRAP_OID`.
- `TRAP_CATEGORY` and `TRAP_SEVERITY`, with a note that they are profile policy.
- Relevant `TRAP_VAR_*` names and sanitized values.
- Receiver metric evidence, such as `received`, `decoded`, `accepted`, `committed`, `dropped`, `write_failed`, `dedup_suppressed`, and the relevant `snmp.trap.errors` dimension.
- A short sanitized summary of `TRAP_JSON` or `TRAP_ENRICHMENT` only when indexed fields do not explain the event.
