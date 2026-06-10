---
name: query-snmp-traps
description: Query SNMP trap logs through Netdata Cloud or directly from a Netdata Agent. Use when the user asks about SNMP traps, trap journal entries, trap severities, trap categories, trap senders, deduplication summaries, TRAP_* fields, TRAP_JSON varbind searches, or how to inspect received traps in the Logs UI/API.
---

# Query SNMP traps

This skill teaches operators and AI assistants how to query SNMP trap
journal entries written by the `snmp_traps` go.d collector.

SNMP trap entries are exposed through the `snmp:traps` Function.
Direct-journal jobs appear as `__logs_sources` options, normally named
after the trap listener job. OTLP-only jobs (`journal.enabled: false`)
do not create local journal files, so they do not appear as log
sources. This skill reuses the token-safe wrappers from
[`query-netdata-agents`](../query-netdata-agents/SKILL.md) and the
Log Function request shape from
[`query-netdata-cloud/query-logs.md`](../query-netdata-cloud/query-logs.md).

## Guides

| Task | How-to |
|---|---|
| Recent security traps from one device | [how-tos/recent-security-traps-from-device.md](./how-tos/recent-security-traps-from-device.md) |
| Critical and emergency traps across a room | [how-tos/filter-by-severity-across-fleet.md](./how-tos/filter-by-severity-across-fleet.md) |
| Top trap senders in the last hour | [how-tos/top-trap-senders-last-hour.md](./how-tos/top-trap-senders-last-hour.md) |
| Dedup summaries during a flap storm | [how-tos/inspect-dedup-summary-entries.md](./how-tos/inspect-dedup-summary-entries.md) |
| Search varbind content in `TRAP_JSON` | [how-tos/search-varbind-value-in-trap-json.md](./how-tos/search-varbind-value-in-trap-json.md) |
| Convert custom MIBs into trap profiles | [how-tos/convert-custom-mibs-to-trap-profiles.md](./how-tos/convert-custom-mibs-to-trap-profiles.md) |
| Operational how-tos catalog | [how-tos/INDEX.md](./how-tos/INDEX.md) |

## Mandatory Requirements

1. **If you analyze, you author a how-to.** When asked a concrete
   SNMP trap query question that is not already covered under
   [`how-tos/`](./how-tos/), author a new how-to and add it to
   [`how-tos/INDEX.md`](./how-tos/INDEX.md) before completing the
   task.
2. **Use token-safe wrappers.** Source
   `docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh`,
   call `agents_load_env`, then use `agents_call_function`,
   `agents_query_cloud`, or `agents_query_agent`. Do not paste raw
   Cloud tokens, agent bearers, or token-bearing curl commands.
3. **Use structured selections first.** The embedded Function is
   scoped to SNMP trap journal files. Use `selections.__logs_sources`
   when the query should target one trap listener job, and usually
   narrow further with `selections.TRAP_REPORT_TYPE=["trap"]` or
   `["deduplication_summary"]`. Use full-text `query` only as a
   residual search over that narrowed result.
4. **Treat trap content as sensitive.** Do not paste raw trap rows,
   SNMP communities, USM secrets, MAC addresses, usernames, public
   device IPs, customer hostnames, or full `TRAP_JSON` payloads into
   durable artifacts. Return summarized fields unless the user
   explicitly needs raw output locally.
5. **Remember the Function is node-scoped.** Cloud-proxied
   `snmp:traps` calls target one node. For a room or fleet query,
   list nodes first and loop over each node's Function, then aggregate
   client-side. Use `__logs_sources` to narrow to a specific listener
   job when needed.

## Trap Field Reference

The collector writes structured fields that are useful for queries:

| Field | Use |
|---|---|
| `MESSAGE` | Rendered human-readable trap description |
| `ND_LOG_SOURCE=snmp-trap` | Fast discriminator for trap entries |
| `TRAP_REPORT_TYPE` | `trap` or `deduplication_summary`; `decode_error_summary` is reserved for future decode-failure reports |
| `TRAP_OID` | Numeric trap OID |
| `TRAP_NAME` | MIB-qualified trap name |
| `TRAP_PDU_TYPE` | `trap` (unacknowledged) or `inform` |
| `TRAP_VERSION` | SNMP version: `v1`, `v2c`, or `v3` |
| `TRAP_CATEGORY` | One of the bounded trap categories |
| `TRAP_SEVERITY` | One of `emerg`, `alert`, `crit`, `err`, `warning`, `notice`, `info`, `debug` |
| `TRAP_SOURCE_IP` | Identified trap source IP |
| `TRAP_SOURCE_UDP_PEER` | UDP peer address |
| `_HOSTNAME` | Source device hostname when resolved by collector identity |
| `ND_NIDL_NODE` | Netdata vnode identity when known |
| `TRAP_DEVICE_VENDOR` | Vendor slug when known |
| `TRAP_INTERFACE` | Topology interface when enrichment is available |
| `TRAP_NEIGHBORS` | Topology neighbors when enrichment is available |
| `TRAP_TAG_*` | Profile/operator labels, selectable but not default facets |
| `TRAP_JSON` | Structured varbind payload; search carefully, avoid faceting on it |
| `TRAP_SUPPRESSED_COUNT` | Dedup summary only |
| `TRAP_SUPPRESSED_FINGERPRINTS` | Dedup summary only |
| `TRAP_REPORT_PERIOD_SEC` | Dedup summary only |

## Standard Setup

Use the wrappers from `query-netdata-agents`:

```bash
source "$(git rev-parse --show-toplevel)/docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh"
agents_load_env
```

Cloud-proxied query, preferred by default:

```bash
NODE_UUID="YOUR_NODE_UUID"
SNMP_TRAPS_FUNCTION="snmp:traps"

agents_call_function \
  --via cloud \
  --node "$NODE_UUID" \
  --function "$SNMP_TRAPS_FUNCTION" \
  --body '{"info":true}'
```

Direct-agent query, for local or Cloud-unavailable cases:

```bash
NODE_UUID="YOUR_NODE_UUID"
AGENT_HOST="agent.example.invalid:19999"
MACHINE_GUID="YOUR_MACHINE_GUID"
SNMP_TRAPS_FUNCTION="snmp:traps"

agents_call_function \
  --via agent \
  --node "$NODE_UUID" \
  --host "$AGENT_HOST" \
  --machine-guid "$MACHINE_GUID" \
  --function "$SNMP_TRAPS_FUNCTION" \
  --body '{"info":true}'
```

## Row Decoding Helper

Log Function responses store rows as arrays. Use `columns` to map row
positions back to field names:

```bash
jq '.columns as $c
    | .data[]? as $row
    | $c
    | to_entries
    | sort_by(.value.index)
    | map({(.key): $row[.value.index]})
    | add' response.json
```

## Source Selection

Start with `{"info":true}` for `snmp:traps` and inspect the
`__logs_sources` required parameter. By default, the SDK selects all
direct-journal sources. To target one listener job, add:

```json
{
  "selections": {
    "__logs_sources": ["local"]
  }
}
```

If a job is missing from `__logs_sources`, verify the job exists and
`journal.enabled` is not `false`. Until Netdata's Function deletion
protocol lands, a node with no direct-journal trap sources may still
show the Function but return no sources or an unavailable response.

## See Also

- [Cloud log Function guide](../query-netdata-cloud/query-logs.md)
- [Direct-agent log Function guide](../query-netdata-agents/query-logs.md)
- [Generic Function invocation through Cloud](../query-netdata-cloud/query-functions.md)
- [Generic direct-agent Function invocation](../query-netdata-agents/query-functions.md)
- [SNMP trap profile format](../../../../src/go/plugin/go.d/config/go.d/snmp.trap-profiles/profile-format.md)
