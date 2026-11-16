# SNMP Trap Field Reference

The collector writes structured fields that are useful for queries:

| Field | Use |
|---|---|
| `MESSAGE` | Rendered human-readable trap description |
| `ND_LOG_SOURCE=snmp-trap` | Fast discriminator for trap entries |
| `TRAP_REPORT_TYPE` | `trap`, `deduplication_summary`, or `decode_error` |
| `TRAP_JOB` | Trap listener job name |
| `TRAP_OID` | Numeric trap OID |
| `TRAP_NAME` | MIB-qualified trap name |
| `TRAP_PDU_TYPE` | `trap` (unacknowledged) or `inform` |
| `TRAP_VERSION` | SNMP version: `v1`, `v2c`, or `v3` |
| `TRAP_CATEGORY` | One of the bounded trap categories |
| `TRAP_SEVERITY` | One of `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` |
| `TRAP_SOURCE_IP` | Identified trap source IP |
| `TRAP_SOURCE_UDP_PEER` | UDP peer address |
| `TRAP_SOURCE_UDP_PORT` | UDP peer source port for decode-error rows |
| `_HOSTNAME` | Source device hostname when resolved by collector identity |
| `TRAP_REVERSE_DNS` | Optional PTR annotation for the source IP; never authoritative identity |
| `ND_NIDL_NODE` | Netdata vnode identity when known |
| `TRAP_DEVICE_VENDOR` | Vendor slug when known |
| `TRAP_INTERFACE` | Topology interface when enrichment is available |
| `TRAP_NEIGHBORS` | Topology neighbors when enrichment is available |
| `TRAP_TAG_*` | Profile/operator labels, selectable but not default facets |
| `TRAP_VAR_*` | Indexed decoded event varbind fields. Enum-backed varbinds use the enum label, with `_RAW` carrying the numeric value. Sensitive and redundant protocol-control varbinds are skipped. |
| `TRAP_ENRICHMENT` | JSON audit trail for source selection and enrichment decisions; search carefully, avoid faceting on it |
| `TRAP_JSON` | Structured payload and audit copy; ordinary trap/decode-error entries can include `netdata_packet_sequence`. Dedup summaries carry aggregate and `by_trap` data. Prefer `TRAP_VAR_*` for normal filtering. |
| `TRAP_SUPPRESSED_COUNT` | Dedup summary only |
| `TRAP_SUPPRESSED_FINGERPRINTS` | Dedup summary only |
| `TRAP_REPORT_PERIOD_SEC` | Dedup summary only |
| `TRAP_DECODE_ERROR_KIND` | Decode-error rows only; bounded failure class |
| `TRAP_DECODE_ERROR` | Decode-error rows only; sanitized decoder error text |
| `TRAP_PACKET_SIZE` | Decode-error rows only; received datagram size |
| `TRAP_PACKET_SHA256` | Decode-error rows only; packet fingerprint without raw bytes |
| `TRAP_LISTENER` | Decode-error rows only; listener endpoint when known |
| `TRAP_ENGINE_ID` | Decode-error rows only; SNMPv3 engine ID when safely extractable |

## Row Decoding

Rows are arrays; `columns` maps field names to row indexes. Capture decoded rows privately because fields such as
`MESSAGE`, `_HOSTNAME`, varbind values and `TRAP_JSON` can contain identifiers or secrets. Choosing columns is not
sanitization. For an already captured response, set its private filename locally:

```bash
TRAP_RESPONSE_FILE="PATH_TO_PRIVATE_RESPONSE_JSON"
TRAP_ROWS_JSON="$(jq '.columns as $c
    | .data[]? as $row
    | $c
    | to_entries
    | sort_by(.value.index)
    | map({(.key): $row[.value.index]})
    | add' "$TRAP_RESPONSE_FILE")"
```

The selected recipe provides a summary projection. Raw inspection remains available when explicitly needed locally;
follow [Safe Execution](./SKILL.md#safe-execution) before displaying or sharing it.

## Implementation Owners

For source verification, the hand-maintained collector guide's
[Journal Field Contract](../../../../src/go/plugin/go.d/collector/snmp_traps/ARCHITECTURE.md#the-journal-field-contract)
and `src/go/plugin/go.d/collector/snmp_traps/internal/output/journal/serialize.go` own emitted fields.
`Serialize`, `addDecodeErrorFields` and `buildTrapJSON` establish the report-specific conditions. The
[profile format](../../../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md) owns indexed-varbind
and operator-profile rules. This reference presents their operator query form.
