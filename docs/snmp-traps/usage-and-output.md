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

When the Agent is connected to Netdata Cloud, direct-journal jobs appear in **Logs** as **SNMP Trap Logs** through the Cloud-required `snmp:traps` Function. Choose the listener job from the source selector when you want one listener, or leave all sources selected when you want all direct-journal jobs on the node.

You can also read direct-journal jobs with normal journal tooling. The per-job root is normally below `/var/log/netdata/traps/<job>/`; the effective `journalctl --directory` path is its machine-id child, such as `/var/log/netdata/traps/<job>/$(tr -d '-' < /etc/machine-id)`. Use [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) for query examples.

If a job is configured as OTLP-only, it does not create local journal files and does not appear as a local SNMP trap log source. Read those trap events in the OTLP receiver or downstream log system.

## Report types

Every trap output row has `TRAP_REPORT_TYPE`. Read that field first because the rest of the row depends on it.

| `TRAP_REPORT_TYPE` | Meaning | First fields to inspect |
|---|---|---|
| `trap` | A decoded Trap or INFORM accepted by the listener and written to the configured output. | `TRAP_NAME`, `TRAP_OID`, `TRAP_SOURCE_IP`, `TRAP_SEVERITY`, `TRAP_CATEGORY`, `MESSAGE`, `TRAP_VAR_*`, `TRAP_JSON` |
| `decode_error` | A packet reached the decode-error path but could not become a normal trap row. | `TRAP_DECODE_ERROR_KIND`, `TRAP_DECODE_ERROR`, `TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_SOURCE_IP`, `TRAP_SOURCE_UDP_PEER`, `TRAP_PACKET_SIZE`, `TRAP_PACKET_SHA256` |
| `deduplication_summary` | Deduplication was enabled and repeated matching traps were suppressed during a summary period. | `TRAP_SUPPRESSED_COUNT`, `TRAP_SUPPRESSED_FINGERPRINTS`, `TRAP_REPORT_PERIOD_SEC`, `TRAP_JSON` |

Drops that happen before the collector writes a row are visible in receiver metrics, not as trap rows. For drop and error counters, see [Metrics](/docs/snmp-traps/metrics.md).

## Reading a normal trap row

For `TRAP_REPORT_TYPE=trap`, read the row in this order, starting with the core triage fields:

| Field | How to use it |
|---|---|
| `TRAP_NAME` | The resolved MIB-qualified trap name. If it is absent, use `TRAP_OID` and `TRAP_CATEGORY=unknown` to investigate profile coverage. |
| `TRAP_OID` | The numeric trap identifier. Use it when the name is unknown, when comparing device documentation, or when writing profile overrides. |
| `TRAP_SOURCE_IP` | The selected source device address after source attribution. This is the main source field for operator triage. |
| `TRAP_SEVERITY` | The profile or override severity. Use it to sort urgency, then confirm with the message and varbinds. |
| `TRAP_CATEGORY` | The profile or override category. Use it to group operational intent. |
| `MESSAGE` | The rendered human-readable trap description. It is the best quick summary, but the structured fields remain the reliable query surface. |

After these, source/peer, host, version, PDU type, and the enrichment fields (`TRAP_SOURCE_UDP_PEER`, `_HOSTNAME`, `TRAP_VERSION`, `TRAP_PDU_TYPE`, `TRAP_DEVICE_VENDOR`, `TRAP_INTERFACE`, `TRAP_NEIGHBORS`, `ND_NIDL_NODE`, `TRAP_REVERSE_DNS`, `TRAP_ENRICHMENT`, `PRIORITY`) refine the picture. For each field's meaning, source, type, and population rule, see [Field Reference](/docs/snmp-traps/field-reference.md).

Severity and category come from trap profiles or operator overrides. They describe the trap meaning; they do not prove the current device state. For state questions, use traps together with polling metrics.

### Worked example: an interface goes down

A switch at `192.0.2.10` sends an `IF-MIB::linkDown` trap to the `edge-traps` listener. With the stock profile pack resolving the OID, the journal row looks like this:

```text
TRAP_REPORT_TYPE=trap
TRAP_OID=1.3.6.1.6.3.1.1.5.3
TRAP_NAME=IF-MIB::linkDown
TRAP_CATEGORY=state_change
TRAP_SEVERITY=warning
TRAP_SOURCE_IP=192.0.2.10
TRAP_VAR_IFINDEX=2
TRAP_VAR_IFADMINSTATUS=up
TRAP_VAR_IFADMINSTATUS_RAW=1
TRAP_VAR_IFOPERSTATUS=down
TRAP_VAR_IFOPERSTATUS_RAW=2
```

`TRAP_CATEGORY` and `TRAP_SEVERITY` are what the stock pack assigns to this OID; a per-OID override can change them. The enum varbinds carry a human label in the main field and the numeric device value in `_RAW`, so `ifOperStatus` reads `down` with `TRAP_VAR_IFOPERSTATUS_RAW=2`.

Find it with the canonical journal query, filtering on the trap OID:

```bash
sudo journalctl \
  --directory=/var/log/netdata/traps/edge-traps/$(tr -d '-' < /etc/machine-id) \
  --since "2 hours ago" \
  TRAP_OID=1.3.6.1.6.3.1.1.5.3 \
  --no-pager
```

On the receiver side, this one event increments the `state_change` dimension of `snmp.trap.events` and the `committed` stage of the pipeline funnel.

## Varbinds and TRAP_JSON

Varbinds are the event-specific data inside the trap. Netdata exposes them in two layers.

| Surface | Use it for |
|---|---|
| `TRAP_VAR_*` | Fast field selection and filtering for non-sensitive, non-redundant event varbinds. |
| `TRAP_JSON` | The structured non-sensitive varbind payload, original varbind OIDs, types, values, enum labels, duplicate keys, and `netdata_packet_sequence`. |

`TRAP_VAR_*` is intentionally not a full varbind dump: it skips redundant protocol-control varbinds (device uptime, trap OID, trap address, trap enterprise) and the sensitive community varbind. Use `TRAP_JSON` when you need the full non-sensitive structure. For the complete `TRAP_VAR_*` naming and skip rules, enum `_RAW` handling, and `TRAP_JSON` shape, see [Field Reference](/docs/snmp-traps/field-reference.md#varbind-fields).

`TRAP_JSON` also includes `netdata_packet_sequence` when the packet sequence is available. This is a per-job receive counter. Use it for receiver-side ordering and troubleshooting; it is not a device timestamp.

## Tags and profile labels

Profiles and operator overrides can attach labels to a trap row. They appear as `TRAP_TAG_*` fields.

Use tags to filter operational groupings that profiles or local overrides define, such as site class, compliance scope, device role, or change-window classification. Tags are separate from core fields like `TRAP_SOURCE_IP`, `TRAP_NAME`, `TRAP_SEVERITY`, and `TRAP_CATEGORY`.

Tag keys are normalized to uppercase after the `TRAP_TAG_` prefix. If a tag key is long, the field name may be shortened.

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

Read these first:

| Field | Meaning |
|---|---|
| `TRAP_DECODE_ERROR_KIND` | Bounded failure class: `malformed_pdu`, `auth_failures`, `usm_failures`, `unknown_engine_id`, or `decode_failed`. |
| `TRAP_DECODE_ERROR` | Sanitized decoder error text. It is shortened and cleaned before storage. |
| `TRAP_SOURCE_IP` | Source address available for the failed packet. |
| `TRAP_PACKET_SHA256` | Packet fingerprint for comparing repeated bad packets without storing raw packet bytes. |

The remaining decode-error and packet-audit fields (`TRAP_CATEGORY`, `TRAP_SEVERITY`, `TRAP_SOURCE_UDP_PEER`, `TRAP_SOURCE_UDP_PORT`, `TRAP_VERSION`, `TRAP_PACKET_SIZE`, `TRAP_LISTENER`, `TRAP_ENGINE_ID`, `TRAP_JSON`) and why raw packet bytes are never stored are in [Field Reference](/docs/snmp-traps/field-reference.md#decode-error-fields).

## Binary-encoded fields

Some field values can contain newlines, NUL bytes, control characters, or invalid UTF-8. Netdata stores those values in a binary-safe form, so a hostile value cannot inject fake fields. Binary encoding does not mean the trap was dropped; the trap was stored normally.

To track how often it happens, watch the `binary_encoded` counter; for its meaning and rate, see [Metrics](/docs/snmp-traps/metrics.md#processing-errors), and for the default alert, see [Alerts](/docs/snmp-traps/alerts.md).

## Receive time vs device uptime

The log row timestamp is the time Netdata received and wrote the trap event. Use that timestamp for incident timelines in Netdata, journalctl, and downstream log systems.

Do not confuse that timestamp with `sysUpTime`. In SNMP traps, `sysUpTime` is device uptime at the time the device generated the trap. It is not wall-clock time, and it is not the receive time. Netdata does not index device uptime as a `TRAP_VAR_*` field because it is a protocol-control varbind; inspect `TRAP_JSON` when you need the original non-sensitive varbind payload.

`netdata_packet_sequence` is also not time. It is a receiver-side per-job counter assigned to UDP datagrams.

## Next steps

- [Field Reference](/docs/snmp-traps/field-reference.md) - Check every field, when it appears, and how to query it.
- [Journal and Querying](/docs/snmp-traps/journal-and-querying.md) - Query trap rows with Netdata Cloud Logs and journal tooling.
- [Metrics](/docs/snmp-traps/metrics.md) - Use receiver metrics, error counters, and dedup counters to explain what rows do not show by themselves.
- [Alerts](/docs/snmp-traps/alerts.md) - See the default health alerts that fire on these receiver metrics.
