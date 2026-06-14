<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/snmp-traps/usage-and-output.md"
sidebar_label: "Usage and Output"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'usage', 'output', 'trap rows', 'varbinds', 'deduplication', 'decode errors']
endmeta-->

<!-- markdownlint-disable-file -->

# Usage and Output

Use this page when a trap row is already visible and you need to understand what happened. Start with the row type, then read the trap name, source, severity, category, message, varbinds, tags, and any summary or decode-error details.

The fastest first pass is:

1. Check `TRAP_REPORT_TYPE`.
2. For a normal trap, read `TRAP_NAME`, `TRAP_SOURCE_IP`, `TRAP_SEVERITY`, `TRAP_CATEGORY`, and `MESSAGE`.
3. Use `TRAP_VAR_*` fields for indexed varbinds.
4. Open `TRAP_JSON` when the row needs full non-sensitive varbind detail, duplicate varbind names, original OIDs, or packet sequence.

## Where to read traps

When the Agent is connected to Netdata Cloud, direct-journal trap jobs appear in **Logs** as **SNMP Trap Logs** through the Cloud-required `snmp:traps` Function. Choose the trap job from the source selector when you want one listener, or leave all sources selected when you want all direct-journal trap jobs on the node.

You can also read direct-journal jobs with normal journal tooling. The per-job root is normally below `/var/log/netdata/traps/<job>/`; the effective `journalctl --directory` path is its machine-id child, such as `/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)`. Use [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) for query examples.

If a job is configured as OTLP-only, it does not create local direct-journal files and does not appear as a local SNMP trap log source. Read those trap events in the OTLP receiver or downstream log system.

## Report types

Every trap output row has `TRAP_REPORT_TYPE`. Read that field first because the rest of the row depends on it.

| `TRAP_REPORT_TYPE` | Meaning | First fields to inspect |
|---|---|---|
| `trap` | A decoded Trap or INFORM accepted by the listener and written to the configured output. | `TRAP_NAME`, `TRAP_OID`, `TRAP_SOURCE_IP`, `TRAP_SEVERITY`, `TRAP_CATEGORY`, `MESSAGE`, `TRAP_VAR_*`, `TRAP_JSON` |
| `decode_error` | A packet reached the decode-error path but could not become a normal trap row. | `TRAP_DECODE_ERROR_KIND`, `TRAP_DECODE_ERROR`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_PACKET_SIZE`, `TRAP_PACKET_SHA256` |
| `deduplication_summary` | Deduplication was enabled and repeated matching traps were suppressed during a summary period. | `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, `TRAP_JSON` |

Drops that happen before the collector writes a row are visible in receiver metrics, not as trap rows. For drop and error counters, see [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md).

## Reading a normal trap row

For `TRAP_REPORT_TYPE=trap`, read the row in this order.

| Field | How to use it |
|---|---|
| `TRAP_NAME` | The resolved MIB-qualified trap name, such as a vendor or standard trap symbol. If it is absent, use `TRAP_OID` and `TRAP_CATEGORY=unknown` to investigate profile coverage. |
| `TRAP_OID` | The numeric trap identifier. Use it when the name is unknown, when comparing device documentation, or when writing profile overrides. |
| `TRAP_SOURCE_IP` | The selected source device address after source attribution. This is the main source field for operator triage. |
| `TRAP_SOURCE_UDP_PEER` | The UDP peer that sent the datagram to Netdata. It can differ from `TRAP_SOURCE_IP` when trusted relay handling selects an original source from the trap payload. |
| `_HOSTNAME` | The source label used for the journal row. Netdata uses the enriched device hostname when available, otherwise `TRAP_SOURCE_IP`, then `TRAP_SOURCE_UDP_PEER`. Treat it as inventory data, not as proof that DNS is authoritative. |
| `TRAP_REVERSE_DNS` | Optional PTR annotation. Use it as a hint only. |
| `TRAP_SEVERITY` | The profile or override severity: `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, or `debug`. Use it to sort urgency, then confirm with the message and varbinds. |
| `PRIORITY` | Journal priority derived from `TRAP_SEVERITY`. Use it when filtering with journal tooling that expects syslog-style priorities. |
| `TRAP_CATEGORY` | The profile or override category: `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, or `unknown`. Use it to group operational intent. |
| `TRAP_PDU_TYPE` | `trap` or `inform`. INFORM means the sender expected an acknowledgement from the receiver. |
| `TRAP_VERSION` | `v1`, `v2c`, or `v3`. Use it when troubleshooting version or authentication policy. |
| `MESSAGE` | The rendered human-readable trap description. It is the best quick summary, but the structured fields remain the reliable query surface. |
| `ND_NIDL_NODE` | Virtual-node identity for the source device when enrichment found one. |
| `TRAP_DEVICE_VENDOR` | Vendor slug when Netdata can identify the source device. |
| `TRAP_INTERFACE` | Interface name from trap varbinds or topology enrichment, when available. |
| `TRAP_NEIGHBORS` | Comma-separated neighbor names from topology enrichment, when available. |
| `TRAP_ENRICHMENT` | JSON audit detail for source attribution and enrichment decisions. Use it for troubleshooting why a source, hostname, vendor, or related field was selected. |

Severity and category come from trap profiles or operator overrides. They describe the trap meaning; they do not prove the current device state. For state questions, use traps together with polling metrics.

## Varbinds and TRAP_JSON

Varbinds are the event-specific data inside the trap. Netdata exposes them in two layers.

| Surface | Use it for |
|---|---|
| `TRAP_VAR_*` | Fast field selection and filtering for non-sensitive, non-redundant event varbinds. |
| `TRAP_JSON` | The structured non-sensitive varbind payload, original varbind OIDs, types, values, enum labels, duplicate keys, and `netdata_packet_sequence`. |

`TRAP_VAR_*` is intentionally not a full varbind dump. Netdata skips protocol-control varbinds that would be redundant as indexed fields, such as device uptime, trap OID, trap address, and trap enterprise. Sensitive community varbinds are skipped. Use `TRAP_JSON` when you need the full non-sensitive structure.

For enum-backed varbinds, the indexed field uses the enum label and a matching `_RAW` field carries the numeric value. For example, an interface status varbind can appear as a readable state in `TRAP_VAR_*` and as the raw number in `TRAP_VAR_*_RAW`.

Long or unusual varbind names are normalized for journal field names. If a field name is shortened, duplicated, or generated from an OID, `TRAP_JSON` still carries the structured varbind entry with its name or OID.

`TRAP_JSON` also includes `netdata_packet_sequence` when the packet sequence is available. This is a per-job receive counter. Use it for receiver-side ordering and troubleshooting; it is not a device timestamp.

## Tags and profile labels

Profiles and operator overrides can attach labels to a trap row. They appear as `TRAP_TAG_*` fields.

Use tags to filter operational groupings that profiles or local overrides define, such as site class, compliance scope, device role, or change-window classification. Tags are separate from core fields like `TRAP_SOURCE_IP`, `TRAP_NAME`, `TRAP_SEVERITY`, and `TRAP_CATEGORY`.

Tag keys are normalized to uppercase after the `TRAP_TAG_` prefix. If a tag key is long, the journal field name may be shortened to stay within journal field-name limits.

## Dedup summaries

When deduplication is enabled, the first matching trap is written normally. Later matching traps inside the dedup window are suppressed and counted. Netdata then writes a summary row with `TRAP_REPORT_TYPE=deduplication_summary`.

Read these fields:

| Field | Meaning |
|---|---|
| `TRAP_SUPPRESSED_COUNT` | Total duplicate trap events suppressed during the summary period. |
| `TRAP_SUPPRESSED_FINGERPRINTS` | Number of distinct dedup fingerprints that had suppressed events. |
| `TRAP_REPORT_PERIOD_SEC` | Length of the summary period in seconds. |
| `MESSAGE` | Human-readable summary, including per-trap counts when available. |
| `TRAP_JSON` | Structured summary with total suppressed count, period, fingerprint count, and counts by trap OID. |

A dedup summary is not a normal trap from one device. It is a job-level summary row for suppressed duplicates. If you expected every repeated PDU to appear as an individual row, check whether deduplication is enabled for that listener job.

## Decode errors

Decode-error rows use `TRAP_REPORT_TYPE=decode_error`. They help you investigate packets that reached the decode-error path but could not be written as normal trap rows.

Read these fields:

| Field | Meaning |
|---|---|
| `TRAP_DECODE_ERROR_KIND` | Bounded failure class: `malformed_pdu`, `auth_failures`, `usm_failures`, `unknown_engine_id`, or `decode_failed`. |
| `TRAP_DECODE_ERROR` | Sanitized decoder error text. It is shortened and cleaned before storage. |
| `TRAP_CATEGORY` | `auth` for SNMPv3 authentication, USM, and unknown-engine failures; otherwise `diagnostic`. |
| `TRAP_SEVERITY` | `warning` for decode-error rows. |
| `TRAP_SOURCE_IP` | Source address available for the failed packet. |
| `TRAP_SOURCE_UDP_PEER` | UDP peer that sent the failed packet. |
| `TRAP_SOURCE_UDP_PORT` | UDP source port, when known. |
| `TRAP_VERSION` | Sniffed SNMP version when Netdata can identify it safely. |
| `TRAP_PACKET_SIZE` | Received datagram size. |
| `TRAP_PACKET_SHA256` | Packet fingerprint for comparing repeated bad packets without storing raw packet bytes. |
| `TRAP_LISTENER` | Listener endpoint that received the packet when known. |
| `TRAP_ENGINE_ID` | SNMPv3 engine ID when safely extractable from the failed packet. |
| `TRAP_JSON` | Structured decode-error details plus `netdata_packet_sequence` when available. |

Raw packet bytes are not written to decode-error rows. Packets can contain communities, credentials, or binary payloads, so Netdata stores sanitized error text and a packet hash instead.

## Binary-encoded fields

Some field values can contain newlines, NUL bytes, control characters, or invalid UTF-8. Netdata writes those values using binary journal encoding so they cannot inject fake fields or corrupt row output.

Binary journal encoding does not mean the trap was dropped. It means the row was stored using the safe journal representation for that field value. If you need to know whether this is happening often, check the `binary_encoded` dimension in the SNMP trap processing errors chart; the metric selector is `snmp_trap_errors_binary_encoded`. See [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md).

## Receive time vs device uptime

The log row timestamp is the time Netdata received and wrote the trap event. Use that timestamp for incident timelines in Netdata, journalctl, and downstream log systems.

Do not confuse that timestamp with `sysUpTime`. In SNMP traps, `sysUpTime` is device uptime at the time the device generated the trap. It is not wall-clock time, and it is not the receive time. Netdata does not index device uptime as a `TRAP_VAR_*` field because it is a protocol-control varbind; inspect `TRAP_JSON` when you need the original non-sensitive varbind payload.

`netdata_packet_sequence` is also not time. It is a receiver-side per-job counter assigned to UDP datagrams.

## Next steps

- [Field Reference](/docs/snmp-traps/field-reference.md) - Check every field, when it appears, and how to query it.
- [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) - Query trap rows with Netdata Cloud Logs and journal tooling.
- [Metrics and Alerts](/docs/snmp-traps/metrics-and-alerts.md) - Use receiver metrics, error counters, dedup counters, and alerts to explain what rows do not show by themselves.
