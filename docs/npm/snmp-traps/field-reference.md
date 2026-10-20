<!--startmeta
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/npm/snmp-traps/field-reference.md"
sidebar_label: "Field Reference"
learn_status: "Published"
learn_rel_path: "SNMP Traps"
keywords: ['snmp traps', 'field reference', 'trap fields', 'journal fields', 'varbinds', 'otlp', 'siem']
endmeta-->

<!-- markdownlint-disable-file -->

# Field Reference

Use this page when you need to map SNMP trap log fields to meaning, source, type, population rules, query use, and sensitive-data handling.

SNMP trap rows are structured logs. Direct-journal jobs expose these fields through the Cloud-required `snmp:traps` Function and through `journalctl --directory=...`. OTLP export sends the same event as an OTLP LogRecord with OTLP attribute names; see [OTLP mapping notes](#otlp-mapping-notes).

## On this page — field index

Saw a field in a log row? Jump straight to its definition. Field families are listed alphabetically; each link lands on the section that defines it.

| Field | Where it is defined |
|---|---|
| `_HOSTNAME` | [Source identity fields](#source-identity-fields) |
| `MESSAGE` | [Report identity fields](#report-identity-fields) |
| `ND_LOG_SOURCE` | [Report identity fields](#report-identity-fields) |
| `ND_NIDL_NODE` | [Source identity fields](#source-identity-fields) |
| `PRIORITY` | [Report identity fields](#report-identity-fields) |
| `SYSLOG_IDENTIFIER` | [Report identity fields](#report-identity-fields) |
| `TRAP_CATEGORY` | [Trap meaning fields](#trap-meaning-fields), [Decode error fields](#decode-error-fields) |
| `TRAP_DECODE_ERROR` | [Decode error fields](#decode-error-fields) |
| `TRAP_DECODE_ERROR_KIND` | [Decode error fields](#decode-error-fields) |
| `TRAP_DEVICE_VENDOR` | [Enrichment fields](#enrichment-fields) |
| `TRAP_ENGINE_ID` | [Packet audit fields](#packet-audit-fields) |
| `TRAP_ENRICHMENT` | [Enrichment fields](#enrichment-fields) |
| `TRAP_INTERFACE` | [Enrichment fields](#enrichment-fields) |
| `TRAP_JOB` | [Report identity fields](#report-identity-fields) |
| `TRAP_JSON` | [Varbind fields](#varbind-fields), [Dedup summary fields](#dedup-summary-fields), [Packet audit fields](#packet-audit-fields) |
| `TRAP_LISTENER` | [Packet audit fields](#packet-audit-fields) |
| `TRAP_NAME` | [Trap meaning fields](#trap-meaning-fields) |
| `TRAP_NEIGHBORS` | [Enrichment fields](#enrichment-fields) |
| `TRAP_OID` | [Trap meaning fields](#trap-meaning-fields) |
| `TRAP_PACKET_SHA256` | [Packet audit fields](#packet-audit-fields) |
| `TRAP_PACKET_SIZE` | [Packet audit fields](#packet-audit-fields) |
| `TRAP_PDU_TYPE` | [Trap meaning fields](#trap-meaning-fields) |
| `TRAP_REPORT_PERIOD_SEC` | [Dedup summary fields](#dedup-summary-fields) |
| `TRAP_REPORT_TYPE` | [Start with report type](#start-with-report-type), [Report identity fields](#report-identity-fields) |
| `TRAP_REVERSE_DNS` | [Source identity fields](#source-identity-fields) |
| `TRAP_SEVERITY` | [Trap meaning fields](#trap-meaning-fields), [Decode error fields](#decode-error-fields) |
| `TRAP_SOURCE_IP` | [Source identity fields](#source-identity-fields) |
| `TRAP_SOURCE_UDP_PEER` | [Source identity fields](#source-identity-fields) |
| `TRAP_SOURCE_UDP_PORT` | [Packet audit fields](#packet-audit-fields) |
| `TRAP_SUPPRESSED_COUNT` | [Dedup summary fields](#dedup-summary-fields) |
| `TRAP_SUPPRESSED_FINGERPRINTS` | [Dedup summary fields](#dedup-summary-fields) |
| `TRAP_TAG_*` | [Profile tag fields](#profile-tag-fields) |
| `TRAP_VAR_*` | [Varbind fields](#varbind-fields) |
| `TRAP_VAR_*_RAW` | [Varbind fields](#varbind-fields) |
| `TRAP_VERSION` | [Trap meaning fields](#trap-meaning-fields), [Decode error fields](#decode-error-fields) |

For OTLP attribute names that map to these journal fields, see [OTLP mapping notes](#otlp-mapping-notes). For the default facets used by the `snmp:traps` Function, see [Default query fields](#default-query-fields).

## Start with report type

Always check `TRAP_REPORT_TYPE` first. It tells you which field set to expect.

| `TRAP_REPORT_TYPE` | Meaning | Fields to expect |
|---|---|---|
| `trap` | A decoded, accepted SNMP Trap or INFORM event. | Trap meaning fields, source fields, optional enrichment fields, optional `TRAP_TAG_*`, optional `TRAP_VAR_*`, and `TRAP_JSON`. |
| `deduplication_summary` | A periodic summary for traps suppressed by deduplication. | `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, and summary-shaped `TRAP_JSON`. Do not expect source, trap OID, trap name, or varbind fields on these rows. |
| `decode_error` | A received packet from an accepted source path failed decode, authentication, USM, or engine-ID handling. | Source fields when known, decode-error fields, packet audit fields, and decode-error-shaped `TRAP_JSON`. Do not expect `TRAP_OID`, `TRAP_NAME`, or `TRAP_VAR_*`. |

## Report identity fields

These fields identify the row and the listener job that produced it.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `MESSAGE` | Human-readable row message. | Collector render path. | string | All report types. | Good for reading rows. Do not use it as a stable grouping key. It can include device, source, or trap text. |
| `PRIORITY` | Syslog priority derived from trap severity. | Collector severity mapping. | integer string | All report types. | Useful when a downstream journal or syslog tool expects syslog priority. In Netdata queries, prefer `TRAP_SEVERITY`. |
| `SYSLOG_IDENTIFIER` | Journal identifier for the listener job. | Job configuration. | string | All report types. | Same operational identity as `TRAP_JOB`. Prefer `TRAP_JOB` for SNMP trap queries. |
| `ND_LOG_SOURCE` | Netdata log source discriminator. | Collector constant. | string | All SNMP trap rows; value is `snmp-trap`. | Use to separate SNMP trap rows from other journal rows. |
| `TRAP_JOB` | Listener job name. | `go.d/snmp_traps.conf`. | string | All report types. | Primary field for per-listener queries and SIEM routing. Job names may encode site or environment names. |
| `TRAP_REPORT_TYPE` | Row type. | Collector. | enum | All report types. | Filter on this first to avoid treating optional fields as always present. |

The Cloud-required `snmp:traps` Function also exposes direct-journal jobs as log sources. Individual journal entries use `ND_LOG_SOURCE=snmp-trap`. OTLP-only jobs do not create local journal files, so they do not appear as local job sources in the `snmp:traps` Function.

## Trap meaning fields

These fields describe what the trap means after profile resolution and operator overrides.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_OID` | Numeric trap OID. | SNMP PDU. | string | `trap` rows only. | Use for exact matching, profile gaps, and vendor-specific trap searches. |
| `TRAP_NAME` | Resolved trap name. | Trap profile or built-in MIB knowledge. | string | `trap` rows when the OID resolves to a name. | Default facet. It can be absent for unknown traps; fall back to `TRAP_OID`. |
| `TRAP_CATEGORY` | Operational category. | Trap profile, override, or decode-error classifier. | enum | `trap` rows; `decode_error` rows when classified. Not on dedup summaries. | Default facet. Values are `state_change`, `config_change`, `security`, `auth`, `license`, `mobility`, `diagnostic`, and `unknown`. |
| `TRAP_SEVERITY` | Operational severity. | Trap profile, override, or decode-error classifier. | enum | `trap` rows; `decode_error` rows when classified. Not on dedup summaries. | Default facet. Values are `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, and `debug`. |
| `TRAP_PDU_TYPE` | SNMP PDU kind. | SNMP PDU. | enum | Decoded trap rows when known. | Values are `trap` and `inform`. Useful when INFORM delivery behavior matters. |
| `TRAP_VERSION` | SNMP protocol version. | SNMP decoder. | enum | Decoded trap rows; decode-error rows when the version can be sniffed. | Values are `v1`, `v2c`, and `v3`. Useful for migration, hardening, and decode-error triage. |

## Source identity fields

These fields identify where the trap came from and how Netdata selected the source identity.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_SOURCE_IP` | Selected trap source IP. | UDP peer, or trusted relay source attribution. | string | `trap` and `decode_error` rows when a source is known. | Default facet. Use for per-device searches, including decode-error triage. Treat source IPs as sensitive in shared examples and exports. |
| `TRAP_SOURCE_UDP_PEER` | Immediate UDP peer address. | UDP packet metadata. | string | `trap` and `decode_error` rows when known. | Use to distinguish direct device delivery from relay delivery. |
| `_HOSTNAME` | Source device hostname, or source address fallback. | Enrichment registry, topology context, or source fallback. | string | Non-dedup rows when a hostname, source IP, or UDP peer is available. | Default facet. Do not assume it came from DNS; check `TRAP_REVERSE_DNS` for PTR annotation. |
| `ND_NIDL_NODE` | Netdata virtual-node identity. | Local Netdata device enrichment. | string | Non-dedup rows when source enrichment finds an unambiguous vnode. | Use when joining trap logs with Netdata node identity. It is optional. |
| `TRAP_REVERSE_DNS` | Reverse-DNS PTR annotation for the source IP. | Optional reverse DNS enrichment. | string | Non-dedup rows when reverse DNS is enabled and a cached lookup is available. | Annotation only. It is not authoritative identity and should not replace `TRAP_SOURCE_IP`. |

## Enrichment fields

These fields add operator context when Netdata can resolve it locally.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_DEVICE_VENDOR` | Device vendor slug. | Local SNMP device registry or topology enrichment. | string | Non-dedup rows when vendor is known. | Default facet. Useful for vendor-specific storms and profile coverage checks. |
| `TRAP_INTERFACE` | Interface associated with the trap. | Local topology context. | string | Non-dedup rows when an interface can be resolved. | Use for interface incident triage. Interface names can reveal network design. |
| `TRAP_NEIGHBORS` | Neighbor names associated with the trap interface. | Local topology context. | string | Non-dedup rows when neighbor context is available. | Useful for L2 impact triage. Neighbor names can be sensitive; avoid public examples. |
| `TRAP_ENRICHMENT` | JSON audit trail for source selection and enrichment decisions. | Collector enrichment audit. | JSON string | Rows where enrichment audit data exists. | Use for debugging why source, vnode, vendor, interface, neighbor, or reverse-DNS fields were or were not applied. Avoid faceting on it and review before forwarding wholesale. |

`TRAP_ENRICHMENT` is for audit/debug. For normal filtering, use the concrete fields above, such as `TRAP_SOURCE_IP`, `_HOSTNAME`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, or `TRAP_NEIGHBORS`.

## Varbind fields

Varbinds are the event-specific payload fields inside the trap. Netdata exposes them in two forms:

- `TRAP_VAR_*` fields for indexed, query-friendly filtering.
- `TRAP_JSON` for the full structured payload and audit copy.

| Field pattern | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_VAR_<NAME>` | Decoded event varbind value. Field names are normalized to uppercase. | SNMP PDU plus trap profile varbind labels. | string | `trap` rows only, for non-sensitive, non-redundant varbinds. | Prefer these fields for normal filtering. Examples: interface index, interface status, vendor event code. Do not assume every trap has the same varbind set. |
| `TRAP_VAR_<NAME>_RAW` | Raw numeric value for an enum-backed varbind. | SNMP PDU plus trap profile enum mapping. | string or integer string | `trap` rows only, when `TRAP_VAR_<NAME>` uses an enum label. | Use when SIEM rules need numeric device values instead of human labels. |
| `TRAP_JSON` | Structured payload JSON. For normal traps, contains non-sensitive varbind entries and `netdata_packet_sequence` when available. For summaries and decode errors, contains the matching summary or decode details. | Collector serialization. | JSON string | All report types. | Use for audit, payload inspection, and residual searches. Prefer `TRAP_VAR_*` for routine filtering. Review before forwarding because varbind values can contain operationally sensitive data. |

`TRAP_VAR_*` naming rules:

| Rule | Behavior |
|---|---|
| Profile names | A profile varbind name becomes `TRAP_VAR_<UPPERCASE_NAME>`. Non-letter and non-digit characters become underscores. |
| OID fallback | If a varbind has no name, the field is based on the numeric OID, for example `TRAP_VAR_OID_...`. |
| Duplicates | Duplicate field bases get numeric suffixes such as `_2`. |
| Long names | Long field names are shortened and include a stable hash suffix. The full varbind name and OID remain available in `TRAP_JSON`. |
| Enum values | The main field uses the enum label. The `_RAW` field carries the raw numeric value. |
| Skipped fields | `TRAP_VAR_*` skips the sensitive `snmpTrapCommunity` varbind and redundant protocol-control varbinds, including `sysUpTime`, `snmpTrapOID`, `snmpTrapAddress`, and `snmpTrapEnterprise`. |

`TRAP_JSON` shape for normal trap rows:

| JSON key | Meaning |
|---|---|
| `netdata_packet_sequence` | Per-job receive counter assigned once per UDP datagram, when available. |
| `<varbind name or OID>` | Object with `oid`, `type`, `value`, and optional `enum`. |
| `<varbind name>#2` | Duplicate key suffix when more than one varbind would use the same JSON key. |

Binary varbind values are represented as hex strings in `TRAP_JSON`. Journal fields with control characters or invalid UTF-8 can be binary-encoded. Treat binary values as payload data, not display text.

Unlike `TRAP_VAR_*`, `TRAP_JSON` keeps non-sensitive protocol-control varbinds such as `sysUpTime`, `snmpTrapOID`, `snmpTrapAddress`, and `snmpTrapEnterprise`. Only the sensitive `snmpTrapCommunity` varbind is omitted from `TRAP_JSON`.

## Profile tag fields

Trap profiles and per-OID overrides can add operator labels. They are exposed as `TRAP_TAG_*`.

| Field pattern | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_TAG_<KEY>` | Profile or override label value. Label keys are uppercased for the journal field name. | Trap profile or job override. | string | Rows where labels are applied. Most commonly `trap` rows. | Use for local policy grouping, such as site class, compliance scope, or ownership. Tags are selectable fields but not default facets. Tag values may reveal internal organization. |

Long tag keys are shortened with a hash suffix.

## Dedup summary fields

Deduplication summary rows are not copies of the suppressed traps. They report suppression activity for a job and period.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_SUPPRESSED_COUNT` | Total traps suppressed in the summary period. | Dedup cache. | integer | `deduplication_summary` rows. | Use to measure repeated-trap storm volume. |
| `TRAP_SUPPRESSED_FINGERPRINTS` | Number of distinct dedup fingerprints suppressed. | Dedup cache. | integer | `deduplication_summary` rows. | Use to tell one repeated event from many repeated event classes. |
| `TRAP_REPORT_PERIOD_SEC` | Summary period length in seconds. | Dedup reporter. | integer | `deduplication_summary` rows. | Use with suppressed count to understand the reporting window. |
| `TRAP_JSON` | Dedup summary JSON with `total_suppressed`, `period_sec`, `fingerprints`, and optional `by_trap`. | Dedup reporter. | JSON string | `deduplication_summary` rows. | Use for audit and per-trap-OID breakdown when present. Avoid treating it like a varbind payload. |

## Decode error fields

Decode-error rows are written for accepted source paths when Netdata can record a safe diagnostic without storing raw packet bytes.

Read this table together with [Packet audit fields](#packet-audit-fields). Packet audit fields such as `TRAP_LISTENER`, `TRAP_ENGINE_ID`, `TRAP_PACKET_SIZE`, `TRAP_PACKET_SHA256`, and `TRAP_SOURCE_UDP_PORT` are also part of decode-error rows.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_DECODE_ERROR_KIND` | Bounded error class. | Decoder classifier. | enum | `decode_error` rows when classified. | Use as the main decode-error facet. Values include `malformed_pdu`, `auth_failures`, `usm_failures`, `unknown_engine_id`, and `decode_failed`. |
| `TRAP_DECODE_ERROR` | Sanitized decoder error text. | Decoder. | string | `decode_error` rows when available. | Useful for local troubleshooting. It is sanitized and shortened, but still avoid pasting operational details into public artifacts. |
| `TRAP_CATEGORY` | Decode-error category. | Decode-error classifier. | enum | `decode_error` rows when classified. | Authentication, USM, and unknown engine-ID failures are categorized as `auth`; other decode failures are usually `diagnostic`. |
| `TRAP_SEVERITY` | Decode-error severity. | Decode-error classifier. | enum | `decode_error` rows when classified. | Decode errors are warning-level diagnostics. |
| `TRAP_VERSION` | Sniffed SNMP version. | Packet sniffer. | enum | `decode_error` rows when the version can be read safely. | Helps separate SNMPv1/v2c malformed packets from SNMPv3 auth or engine-ID problems. |

## Packet audit fields

Packet audit fields appear on decode-error rows. They help troubleshoot without writing raw packet bytes.

| Field | Meaning | Source | Type | Populated when | Query use and cautions |
|---|---|---|---|---|---|
| `TRAP_PACKET_SIZE` | Received datagram size in bytes. | UDP packet metadata. | integer | `decode_error` rows. | Use to spot oversized or truncated payload patterns. |
| `TRAP_PACKET_SHA256` | SHA-256 fingerprint of the received datagram. | Packet digest. | hex string | `decode_error` rows. | Use to group repeated bad packets without storing raw bytes. It is a fingerprint, not packet content. |
| `TRAP_LISTENER` | Local listener endpoint that received the packet. | Listener socket metadata. | string | `decode_error` rows when known. | Useful when a job binds multiple endpoints. It can reveal local bind addresses. |
| `TRAP_ENGINE_ID` | SNMPv3 engine ID extracted from a failed packet when safely available. | SNMPv3 packet inspection. | hex string | `decode_error` rows when extractable. | Not an auth or privacy secret, but it is a device identifier. Treat it as sensitive inventory data. |
| `TRAP_SOURCE_UDP_PORT` | UDP source port. | UDP packet metadata. | integer | `decode_error` rows when known. | Decode-error rows only; not present on normal trap rows. |
| `TRAP_JSON` | Decode-error details JSON. | Collector serialization. | JSON string | `decode_error` rows. | Contains fields such as kind, error, packet size, packet hash, source port, listener, SNMP version, engine ID, and packet sequence when available. Do not confuse it with normal trap varbind JSON. |

Raw packet bytes are not stored in decode-error rows because SNMP communities and binary payloads can appear inside received datagrams.

## Default query fields

The `snmp:traps` Function uses these default facets:

| Field | Use |
|---|---|
| `TRAP_CATEGORY` | Group by operational category. |
| `TRAP_DEVICE_VENDOR` | Group by vendor when enrichment is available. |
| `TRAP_NAME` | Group by resolved trap name. |
| `TRAP_SEVERITY` | Group by operational severity. |
| `TRAP_SOURCE_IP` | Group by selected source device address. |
| `_HOSTNAME` | Group by resolved or fallback host identity. |
| `TRAP_JOB` | Group by listener job. |

Recommended query pattern:

1. Filter `TRAP_REPORT_TYPE`.
2. Narrow with `TRAP_JOB`, `TRAP_SOURCE_IP`, `_HOSTNAME`, `TRAP_NAME`, `TRAP_OID`, `TRAP_CATEGORY`, or `TRAP_SEVERITY`.
3. Use `TRAP_VAR_*` only after you know that the selected trap type emits that varbind.
4. Inspect `TRAP_JSON` or `TRAP_ENRICHMENT` only when the indexed fields do not answer the question.

For examples, see [Usage and output](/docs/npm/snmp-traps/usage-and-output.md) and [Journal and querying](/docs/npm/snmp-traps/journal-and-querying.md).

## Sensitive-data cautions

Treat trap logs as operational event data. They can contain sensitive inventory, network, security, and user context.

| Data | What Netdata does | Operator caution |
|---|---|---|
| SNMPv1/v2c community varbind | `snmpTrapCommunity` is omitted from `TRAP_VAR_*`, `TRAP_JSON`, and OTLP varbind payloads. | Do not paste real community strings from configs, packet captures, or device CLIs into examples or tickets. |
| SNMPv3 auth and privacy values | Auth keys and privacy keys are configuration secrets and are not emitted as trap log fields. | Use Netdata secret references in configuration. Do not include resolved values in logs, docs, SIEM examples, or support artifacts. |
| `TRAP_JSON` | Stores structured payloads and audit details. Sensitive community varbinds are skipped, but other varbinds can still contain usernames, interface descriptions, MACs, public IPs, asset tags, locations, or vendor text. | Prefer specific `TRAP_VAR_*` fields for rules. Review and minimize before forwarding, indexing, or sharing full payloads. |
| `TRAP_VAR_*` | Exposes query-friendly event varbinds. | Treat values as device-provided payload. Do not assume they are safe for public examples. |
| `TRAP_ENRICHMENT` | Records source and enrichment decisions. | Can include hostnames, source addresses, interface names, neighbor names, and applied fields. Use for debugging, not broad faceting. |
| `TRAP_ENGINE_ID` | Exposes SNMPv3 engine ID when safely extracted from decode-error packets. | Not a password, but it is an inventory identifier. Avoid public examples with real values. |
| Binary values | `TRAP_JSON` encodes byte values as hex strings; journal fields can be binary-encoded when needed. | Do not display or forward binary payloads without reviewing data classification. |
| OTLP headers | Header values can use Netdata secret references in configuration. | Header values are transport credentials, not trap fields. Protect them like secrets. |

## OTLP mapping notes

When `otlp.enabled` is `true`, Netdata exports traps as OTLP LogRecords. OTLP uses attribute names, not journal field names.

| Journal concept | OTLP location | Notes |
|---|---|---|
| Listener job | Resource attribute `service.instance.id`; resource `service.name` is `netdata-snmptrap`. | Use `service.instance.id` as the OTLP equivalent of `TRAP_JOB`. |
| Message | Log body. | Equivalent to `MESSAGE`. |
| Report type | Attribute `snmp.trap.report_type`. | Values match `TRAP_REPORT_TYPE`. |
| Event name | LogRecord event name. | Normal traps use `snmp.trap.<category>`. Dedup summaries use `snmp.trap.deduplication_summary`. Decode errors use `snmp.trap.decode_error`. |
| Severity | OTLP severity number/text and attribute `snmp.trap.severity`. | The attribute carries the Netdata severity slug. |
| Source IP | Attribute `snmp.source.ip`. | Equivalent to selected `TRAP_SOURCE_IP`; OTLP falls back to the UDP peer when the selected source IP is empty. |
| UDP peer | Attribute `network.peer.address`. | Similar to `TRAP_SOURCE_UDP_PEER`, but OTLP falls back to the selected source IP when the UDP peer is empty. |
| UDP source port | Attribute `network.peer.port`. | Decode-error rows only. |
| Trap OID and name | Attributes `snmp.trap.oid` and `snmp.trap.name`. | Normal trap rows only. |
| Category and PDU type | Attributes `snmp.trap.category` and `snmp.trap.pdu_type`. | Normal trap rows; decode errors may carry category. |
| SNMP version | Attribute `snmp.version`. | Normal trap rows and decode-error rows when known. |
| Device identity and enrichment | Attributes `snmp.device.hostname`, `snmp.device.vendor`, `netdata.nidl.node`, `netdata.topology.interface`, `netdata.topology.neighbors`, and `snmp.source.reverse_dns`. | Populated only when the matching journal enrichment field is available. |
| Profile tags | Attributes named `trap.<lowercase key>`. | Equivalent to `TRAP_TAG_*`. |
| Varbind payload | Attribute `snmp.varbinds`. | OTLP does not export separate `TRAP_VAR_*` fields. For normal traps, this is the structured varbind list. For decode-error rows, it contains decode diagnostic details. For dedup-summary rows, it contains summary counters. Sensitive community varbinds are omitted. |
| Dedup summary | Attributes `snmp.trap.suppressed_count`, `snmp.trap.suppressed_fingerprints`, and `snmp.trap.report_period_sec`; summary details in `snmp.varbinds`. | Equivalent to dedup summary journal fields. |
| Decode error and packet audit | Attributes `snmp.trap.decode_error.kind`, `snmp.trap.decode_error.message`, `snmp.trap.packet_size`, `snmp.trap.packet_sha256`, `netdata.trap.listener`, and `snmp.engine_id`; details also in `snmp.varbinds`. | Equivalent to decode-error and packet audit journal fields. |

When forwarding to a SIEM, decide whether the SIEM ingests direct-journal fields or OTLP attributes. Build rules against the field names that actually arrive in that system. See [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md).

## What's next

- [Journal and querying](/docs/npm/snmp-traps/journal-and-querying.md) - Query direct-journal trap rows locally or through Netdata.
- [Forwarding to SIEM](/docs/npm/snmp-traps/forwarding-to-siem.md) - Forward trap events and map fields in an external log system.
