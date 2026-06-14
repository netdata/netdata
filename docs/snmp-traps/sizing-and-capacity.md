<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/sizing-and-capacity.md"
sidebar_label: "Sizing and Capacity"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'sizing', 'capacity planning', 'storm control', 'otlp', 'journal']
endmeta-->

<!-- markdownlint-disable-file -->

# Sizing and Capacity Planning

Use this page when planning an SNMP trap receiver for an SRE team, platform owner, or NetOps lead. The goal is to choose a receiver host and configuration that can absorb the real trap pattern from your devices, keep the history you need, and show when the receiver is dropping, suppressing, or failing to write trap rows.

There is no universal trap-per-second number that applies to every deployment. Capacity depends on packet shape, SNMP version, profile work, storm controls, output backend, storage policy, enabled metrics, and host resources. Size the shipped receiver by testing representative traffic in a lab or staging environment, then adjust configuration while watching receiver metrics and host metrics.

For the configuration keys used here, see [Configuration](/docs/snmp-traps/configuration.md). For receiver metrics, see [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md). For validation workflow and quality checks, see [Validation and Data Quality](/docs/snmp-traps/validation-and-data-quality.md) and [Anti-patterns](/docs/snmp-traps/anti-patterns.md).

## Start with the operating model

Before changing limits, write down the operating model for each listener job:

- Which devices send traps directly?
- Which relays send on behalf of other devices?
- Which SNMP versions and credentials are expected?
- Which output is the system of record: local journal, OTLP, or both?
- How long must local trap history remain available for incident review?
- Which trap OIDs must be classified by profiles?
- Which traps, if any, should become profile-defined metrics?
- Which storm policy is acceptable for each source: keep all traffic, sample over-limit traffic, or drop over-limit traffic?

Keep separate listener jobs when the answers differ by network zone, relay, security policy, or output backend. A smaller, clearer job is easier to validate than one broad job with mixed requirements.

## Sizing inputs

Use these inputs together. A receiver that is fine for steady low-volume traps may fail during a short burst, and a receiver that is fine for journal-only storage may behave differently when OTLP export is also enabled.

| Input | Why it matters | What to check |
|---|---|---|
| Trap rate and burst shape | UDP bursts can arrive faster than the receiver, output backend, or disk can absorb them. The short burst shape matters as much as the long-term average. | Test normal traffic, maintenance-window traffic, reboot storms, link flap storms, and relay replay behavior. |
| Packet and varbind shape | Hard decode limits protect the UDP-exposed parser. Large, malformed, deeply nested, or unusually wide PDUs cost capacity differently from normal traps and may become decode-error rows instead of normal trap rows. | Validate maximum vendor packet size, varbind count, and large text fields. The shipped decode limits are 8 KiB per datagram, 256 varbinds per PDU, BER nesting depth 8, encoded OID length 128 bytes, and OctetString value length 1024 bytes. |
| Number of sending devices and relays | More senders increase source tracking, policy checks, and per-source visibility work. Relays can hide many devices behind fewer UDP peers unless source attribution is configured correctly. | Count direct device sources, relay peers, and expected original sources behind each relay. Validate `source.trusted_relays` only for known relays. |
| SNMP versions and auth cost | SNMPv1/v2c parsing is cheaper than SNMPv3 authentication and privacy processing. SNMPv3 engine ID policy also adds state and failure modes. | Enable only the versions devices use. For v3, validate USM users, auth/privacy protocols, engine IDs, and dynamic engine ID limits. |
| SNMPv3 dynamic engine ID capacity | Dynamic engine ID discovery stores a bounded number of `(engineID, username)` pairs per job. If the registry fills, new dynamic pairs are rejected with `unknown_engine_id` and `dropped`. First-time accepted registrations also increment `unknown_engine_id`, so a small onboarding spike can be normal when `accepted` and `committed` continue to rise. | If dynamic discovery is enabled, compare expected distinct `(engineID, username)` pairs with `dynamic_engine_id_max_pairs` (default `4096`) and watch `unknown_engine_id`, `dropped`, `accepted`, and `committed` while onboarding devices or testing relays. |
| Profile coverage and template complexity | Every accepted trap needs profile lookup. Matching profiles and rendering templates can expose unknown OIDs or unresolved templates. | Watch `unknown_oid`, `template_unresolved`, `accepted`, and `committed` while sending known device traps. |
| Dedup and rate-limit policy | Storm controls change both capacity and semantics. They can protect storage and downstream systems, but they can also hide individual repeated rows by design. | Decide per source whether over-limit traffic should be dropped or sampled, and whether repeated matching traps should be summarized. |
| Journal vs OTLP vs both | Journal writes use local disk and provide local querying through journalctl and the `snmp:traps` Function. OTLP export uses queueing, network, and downstream receiver capacity. Both enabled means both paths must be healthy. | Validate `committed`, `write_failed`, `journal_write_failed`, and `otlp_export_failed` for the selected backend mode. |
| Local retention duration and size | Direct journal retention controls how much local history remains available and how much disk is reserved for trap rows. | Choose `retention.max_size` and optional age limits from the incident-review window and disk budget. |
| Profile-defined metrics and cardinality | Profile metrics create time-series only for committed traps. Rules with unbounded labels or resource identity can create too many metric instances. | Keep profile metrics disabled until selected rules are reviewed. Watch profile metric diagnostics and cardinality caps. |
| Source metric caps and per-source visibility | Per-source charts are bounded. Hitting the source cap limits per-source visibility, but accepted traps can still be committed. Profile-defined metrics have a separate source cap. | Watch active sources, source attribution, `overflow_dropped`, source pipeline, source-attributed errors, and profile metric diagnostics. |
| Host CPU, memory, disk I/O, free page cache, and network | Decode, auth, templates, queues, journal writes, and OTLP export all compete for host resources. Free page cache helps absorb file I/O. | Watch Netdata host charts for CPU, memory pressure, disk utilization, disk latency, page cache, UDP errors, and interface drops. |
| UDP receive buffer | The listener requests a UDP receive buffer per bound endpoint. A buffer can absorb short bursts, but it cannot fix sustained overload. The operating system may grant less than the requested value, depending on kernel limits. | The default `listen.receive_buffer` is `4194304` bytes, or 4 MiB, per bound endpoint. The configured maximum is `268435456` bytes, or 256 MiB. A job with multiple endpoints requests the buffer for each endpoint. Set `0` only when you want the operating-system default, which may be lower than Netdata's default unless the host is tuned. |

## Useful shipped defaults

These defaults are useful starting points, not capacity promises:

- `listen.receive_buffer`: `4194304` bytes, or 4 MiB, requested per bound endpoint.
- `dynamic_engine_id_max_pairs`: `4096` dynamic SNMPv3 `(engineID, username)` pairs per job when dynamic engine ID discovery is enabled.
- `rate_limit.per_source_pps`: `1000` when rate limiting is enabled and `per_source_pps` is unset or `0`.
- `rate_limit.mode`: `drop` when rate limiting is enabled and `mode` is unset.
- Rate limiter source buckets: `10000` active source IPs per job. This cap is fixed and is not a configuration key.
- `dedup.window_sec`: `5` seconds when deduplication is enabled and no window is set.
- `dedup.cache_max_entries`: `100000` fingerprints per job when deduplication is enabled and no cache size is set.
- `journal.enabled`: `true` for explicit jobs.
- Journal writer queue: `10000` in-process trap entries per job. This cap is fixed and is not a configuration key.
- Journal writer flush: up to `1000` entries or `1s`, whichever comes first. These values are fixed and are not configuration keys.
- `retention.max_size`: `10GB` for direct journal storage. The default is 10,000,000,000 bytes.
- `retention.max_duration`: `null`, so age-based deletion is disabled unless configured.
- `retention.rotation_size`: `null`, which means automatic rotation when a journal file reaches `max_size / 20`, clamped between `5MiB` and `200MiB`. With the default `max_size`, the computed value exceeds the clamp, so the effective automatic rotation size is `200MiB`.
- `retention.rotation_duration`: `null`, so time-based rotation is disabled unless configured.
- `otlp.enabled`: `false`.
- `otlp.endpoint`: `http://127.0.0.1:4317`.
- `otlp.request_timeout`: `5s`.
- `otlp.flush_interval`: `200ms`.
- `otlp.batch_size`: `512`.
- `otlp.queue_capacity`: `10000`.
- `profile_metrics.enabled`: `false`.
- `profile_metrics.limits.max_rules`: `500`.
- `profile_metrics.limits.max_sources`: `2000`.
- `profile_metrics.limits.max_resources_per_source`: `512`.
- `profile_metrics.limits.max_instances_per_job`: `50000`.
- `profile_metrics.limits.overflow`: `drop_and_count`, so over-cap metric instances are skipped and counted while the accepted trap can still be committed.

Review the complete option list in [Configuration](/docs/snmp-traps/configuration.md) before changing defaults.

## Storm behavior

Trap storms are operational events. The receiver exposes whether it received, decoded, accepted, committed, suppressed, dropped, or failed to write trap rows.

Important behavior:

- `rate_limit` is disabled by default. When enabled, it is per source.
- `rate_limit.mode: drop` increments `rate_limited` and discards over-limit traps.
- `rate_limit.mode: sample` lets over-limit traps continue through the pipeline but counts rate-limited events.
- Rate limiting tracks up to 10,000 active source IP buckets per job. Idle buckets are swept no more often than every 5 minutes; buckets with no activity for more than 10 minutes are removed. If the cap is still reached, the oldest remaining bucket is evicted before the new source is admitted.
- When both rate limiting and deduplication are enabled, rate limiting is applied first. In `drop` mode, an over-limit trap is discarded before deduplication. In `sample` mode, an over-limit trap increments `rate_limited`, continues through the pipeline, and can still be dedup-suppressed.
- Netdata attempts INFORM responses before the rate-limit gate. In `drop` mode, a rate-limited INFORM can receive a response while its trap row is not journaled or committed.
- `dedup` is disabled by default. When enabled, repeated matching traps inside the deduplication window are summarized instead of being written one by one.
- Dedup-suppressed traps do not update profile-defined metrics.
- Dedup summary rows are written as `TRAP_REPORT_TYPE=deduplication_summary` rows so operators can see that repeated traps were suppressed.
- `write_failed` means the authoritative output path could not keep the trap. The usual capacity case is writer rejection before enqueueing, such as a full queue, closed writer, or writer already in a failed state. In OTLP-only jobs, an asynchronous export failure after enqueueing also increments pipeline `write_failed`. In journal-and-OTLP jobs, asynchronous OTLP export failures are reported as `otlp_export_failed`; they do not make the local journal write fail.
- If the authoritative writer queue is full, the trap is rejected by that writer, counted as `write_failed`, and not committed. The authoritative writer is the journal for journal-backed jobs and OTLP for OTLP-only jobs. In these authoritative paths, pipeline `write_failed` pairs with `journal_write_failed` or `otlp_export_failed` respectively. In journal-and-OTLP jobs, OTLP secondary queue or export failures are reported as `otlp_export_failed` only; they do not increment `write_failed`, do not increment `dropped`, and do not make the local journal write fail.
- `journal_write_failed` points to the local direct journal write path.
- `otlp_export_failed` points to the OTLP export path.
- OTLP-only jobs do not create local journal files and have no local journal backstop. In OTLP-only mode, `committed` means the trap was queued for OTLP export. If asynchronous export later fails, Netdata increments the `otlp_export_failed` error dimension and the pipeline `write_failed` counter; the trap is not retained locally. Use OTLP-only only when the external OTLP receiver is the intended system of record.

During a storm, inspect pipeline dimensions. Pipeline dimensions are receiver outcome counters; error dimensions are diagnostic counters. They are not mutually exclusive, so one trap can increment both a pipeline dimension such as `dropped` and a specific error dimension such as `rate_limited`.

- `received`
- `decoded`
- `accepted`
- `committed`
- `write_failed`
- `dropped`
- `dedup_suppressed`

Also inspect error dimensions:

- `rate_limited`
- `dropped_allowlist`
- `decode_failed`
- `malformed_pdu`
- `auth_failures`
- `usm_failures`
- `unknown_engine_id`
- `unknown_oid`
- `template_unresolved`
- `profile_load_failed`
- `inform_response_failed`
- `binary_encoded`
- `listener_read_failed`
- `otlp_export_failed`
- `journal_write_failed`

`dropped` is the broad counter for traps that ended without a committed normal trap row, a dedup-suppression result, or an authoritative write-failed outcome. It includes allowlist and version drops, decode failures and malformed packets, authentication and USM failures, unknown engine IDs, rate-limit `drop` mode discards, and packet-handling failures. Decode-error rows can increment `dropped` in parallel with their specific error dimension. Use the error dimensions to identify which path is responsible.

Allowlist drops can happen before decode, from source or version checks, and after decode, from community or version checks. Both paths increment `dropped_allowlist` and `dropped`.

`binary_encoded` only increments on direct-journal output paths. OTLP-only jobs keep it at zero because they have no local journal writer.

If `received` rises but `decoded` does not, check pre-decode paths: source allowlists, version sniffing, malformed packets, authentication or USM failures, and decode failures. If `decoded` rises but `accepted` does not, check post-decode version or community allowlists, SNMPv3 engine ID policy, dynamic engine ID registration, and rate limits. Profile coverage gaps and template rendering errors are non-blocking: they increment error counters but do not prevent acceptance, so they do not explain an accepted-counter gap. If `accepted` rises but `committed` does not, check deduplication and the authoritative output backend: local journal health for journal-backed jobs, or OTLP queue/export health for OTLP-only jobs. In journal-and-OTLP jobs, asynchronous OTLP export failures are reported as `otlp_export_failed`; they do not lower `committed`.

## Output and retention planning

Choose the output mode first:

- **Journal only**: good when local Logs / `snmp:traps` is the main investigation path. Size disk for retention and watch journal write failures.
- **OTLP only**: good when the external OTLP receiver is the system of record. There is no local journal backstop and no local job source in the `snmp:traps` Function for that job.
- **Journal and OTLP**: good when operators need local query plus downstream export. Both backend paths must be validated.

At least one output backend must be enabled. OTLP is disabled by default; enabling OTLP adds downstream export and does not disable the journal unless `journal.enabled: false` is also set.

For direct journal jobs, choose retention from the incident-review window and disk budget. The default direct journal retention is `max_size: 10GB` per job. Increase or decrease it based on real row volume, not on device count alone. The journal writer flushes up to 1,000 entries or every 1 second, whichever comes first. Job creation fails if direct journal storage is enabled and the configured Netdata log directory is missing or not writable.

Watch local storage during validation:

- available disk space;
- disk write latency;
- disk utilization;
- filesystem errors;
- free page cache;
- journal rotation behavior;
- `journal_write_failed`.

For OTLP jobs, validate the downstream receiver too. Watch queue behavior through `otlp_export_failed`, downstream ingestion errors, and the records that arrive at the OTLP destination. If OTLP is enabled, job startup performs a preflight export to the configured endpoint using `otlp.request_timeout`. If the receiver is unreachable during startup, the job does not start.

## Profile metrics and cardinality

Profile-defined metrics are disabled by default. Enable them only after you know which trap-to-metric rules are useful and bounded.

Capacity risks come from identity and labels:

- A rule that creates one metric per interface is bounded by the number of interfaces per source.
- A rule that uses an unbounded text value as a resource label can create uncontrolled metric instances.
- A rule that uses raw source labels can expose source values in metrics. Keep the default hashed source identity unless raw labels are acceptable for your environment.
- Dedup-suppressed traps and write-failed traps do not update profile-defined metrics.

Use the shipped caps as guardrails:

| Limit | Default | Meaning |
|---|---:|---|
| `max_rules` | `500` | Maximum enabled metric rules evaluated by the job. |
| `max_sources` | `2000` | Maximum non-listener source identities tracked by the job. |
| `max_resources_per_source` | `512` | Default resource cap per source and resource class. |
| `max_instances_per_job` | `50000` | Maximum profile-derived metric instances for the job. |

Individual profile-derived charts default to a 2,000-instance cap per chart. Profile rules can override this with `chart_meta.lifecycle.max_instances`; over-cap chart instances are skipped and counted by profile metric overflow diagnostics.

If a cap is reached, over-cap metric instances are skipped and counted, while the accepted trap can still be committed. Keep the receiver pipeline and profile metric diagnostics separate when deciding whether trap storage is healthy.

Rules can set `identity.resource.max_per_source` for a specific resource class. The `profile_metrics.limits.max_resources_per_source` default applies when a rule does not set its own resource cap.

## Source visibility

Per-source charts help find noisy senders and source-specific failures, but they are bounded by source metric caps. High source count can be normal in large networks; sudden growth usually needs investigation.

Receiver per-source chart tracking is capped at 2,000 active sources per job. Inactive sources expire after 60 collection cycles of this job. At the default `update_every: 1`, this is 60 seconds; if `update_every` changes, the wall-clock expiry scales proportionally. The receiver cap and expiry are fixed. The `active` dimension on the SNMP trap active sources chart is an absolute count that rises and falls as sources appear and expire. If the cap is reached, accepted traps can still be committed, but per-source chart visibility is limited and `overflow_dropped` rises. The 60-cycle expiry applies to receiver per-source charts. Profile-defined metrics use a separate configurable source cap, `profile_metrics.limits.max_sources`, which also defaults to 2,000.

Use the source chart families together:

- `active sources` shows the current number of tracked sources.
- `source attribution` shows whether source identity came from a vnode, fallback source, or ambiguous source.
- `source pipeline` shows per-source accepted, committed, write-failed, and dedup-suppressed activity.
- `source-attributed errors` shows per-source profile, template, and backend-write errors.
- `source last seen` shows how recently each tracked source sent traps.

Use per-source visibility to answer:

- Which sources are active?
- Which sources are being dedup-suppressed?
- Which sources are failing backend writes?
- Which sources have profile or template errors?
- Did attribution use a vnode, a fallback source, or an ambiguous source?
- Did source tracking hit `overflow_dropped`?

When relays are involved, validate source attribution with trap rows and metrics together. The UDP peer should be the relay, such as `192.0.2.10`, and the selected source should become the original device address, such as `198.51.100.20`, only when the relay is trusted.

## Capacity validation workflow

Validate capacity before pointing production devices at the receiver.

1. Build a lab or staging job that matches production configuration as closely as possible.
2. Generate representative trap traffic from test devices, replay tools, or controlled senders.
3. Include normal flow, expected maintenance bursts, relay behavior, SNMPv3 traffic if used, unknown OIDs, and repeated traps that should exercise deduplication.
4. Watch receiver pipeline dimensions: `received`, `decoded`, `accepted`, `committed`, `write_failed`, `dropped`, and `dedup_suppressed`.
5. Watch error dimensions: `rate_limited`, `dropped_allowlist`, `decode_failed`, `malformed_pdu`, `auth_failures`, `usm_failures`, `unknown_engine_id`, `unknown_oid`, `template_unresolved`, `profile_load_failed`, `inform_response_failed`, `binary_encoded`, `listener_read_failed`, `journal_write_failed`, and `otlp_export_failed`.
6. Watch source charts: active sources, source attribution, source pipeline, source-attributed errors, and source last seen.
7. Watch host charts for CPU, memory, disk I/O, disk latency, free page cache, UDP socket pressure, packet drops, and network interface drops.
8. Confirm the selected system of record has the rows you need: local journal / `snmp:traps` Function, downstream OTLP receiver, or both.
9. Adjust one configuration area at a time, then repeat the same traffic run.

Common adjustments:

- Narrow `versions`, `communities`, `usm_users`, and `allowlist.source_cidrs` to expected senders and credentials.
- Increase host CPU, memory, disk I/O capacity, or free disk space when host charts show saturation.
- Increase `listen.receive_buffer` when short UDP bursts exceed socket buffering and the host has enough memory.
- Use `rate_limit.mode: sample` during validation when you need to measure storm volume without discarding over-limit traps.
- Use `rate_limit.mode: drop` when protecting storage or downstream export is more important than keeping every over-limit trap.
- Enable `dedup` when repeated matching traps should be summarized.
- Add bounded `dedup.key_varbinds` when one trap OID represents distinct resources that must not suppress each other.
- Keep profile metrics disabled, or use exact rule selection, until metric identity and cardinality are reviewed.
- Increase or decrease direct journal retention from the required local investigation window.
- Avoid OTLP-only mode unless downstream OTLP storage is reliable enough to be the system of record.

Capacity validation is complete when the representative traffic pattern produces the expected rows, the expected suppression or drop behavior, no unexpected write failures, and acceptable host resource usage.
