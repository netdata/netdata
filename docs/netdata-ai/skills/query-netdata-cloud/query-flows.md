# Query network-flow Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.
For the generic Function transport, see
[query-functions.md](./query-functions.md).

Flow Functions return network-flow records (NetFlow / sFlow /
IPFIX) ingested by the agent's flow collector. Their dataset is
table-shaped (one row per flow tuple) AND time-windowed AND
faceted, sitting between table snapshots (`processes`) and log
queries (`systemd-journal`).

---

## Function names registered today

Verified live and in source:

| Function | Source crate | Layer | What it returns |
|---|---|---|---|
| `flows:netflow` | `src/crates/netflow-plugin/` | L3 | Network flow records ingested via NetFlow v5/v9, IPFIX, sFlow |

The `flows:` prefix is the canonical namespace; only `netflow` is
registered today. The Function name covers all three protocols
(the collector parses NetFlow, IPFIX, and sFlow into a single
record schema).

---

## Endpoint and request

Standard Cloud Function-call endpoint:

`POST /api/v2/nodes/{nodeId}/function?function=flows:netflow`

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "mode":     "flows",
  "view":     "table-sankey",
  "after":    -3600,
  "before":   0,
  "group_by": ["SRC_AS_NAME", "PROTOCOL", "DST_AS_NAME"],
  "sort_by":  "bytes",
  "top_n":    100
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=flows:netflow" \
  -d "$PAYLOAD"
```

### Modes

The Function has three modes selected by the `mode` body field:

| Mode | Purpose |
|---|---|
| `flows` (default) | Return flow records / aggregations / charts |
| `autocomplete` | Return values for a single facet field, given a search prefix |

### Body parameters

Verified against `src/crates/netflow-plugin/src/api/flows/handler.rs`:

| Parameter | Used in mode | Description |
|---|---|---|
| `mode` | both | `flows` or `autocomplete` |
| `view` | flows | One of: `table-sankey`, `timeseries`, `country-map`, `state-map`, `city-map` |
| `after` | flows | Unix seconds, lower bound. Negative = relative seconds from `before` |
| `before` | flows | Unix seconds, upper bound. `0` = now |
| `query` | flows | Free-text filter |
| `selections` | flows | Pre-applied facet filters as `{ "FIELD_NAME": ["val", "val2"] }`. Common fields: `SRC_ADDR`, `DST_ADDR`, `SRC_PORT`, `DST_PORT`, `PROTOCOL`, `SRC_AS_NAME`, `DST_AS_NAME`, `SRC_COUNTRY`, `DST_COUNTRY`, `INTERFACE`, ... |
| `facets` | flows | Array of facet field names whose value-distributions should appear in the response |
| `group_by` | flows | Up to 10 tuple-key field names (e.g. `["SRC_ADDR","DST_ADDR","PROTOCOL"]`) -- order defines the aggregation tuple |
| `sort_by` | flows | `bytes` or `packets` |
| `top_n` | flows | One of `25`, `50`, `100`, `200`, `500` |
| `field` | autocomplete | Facet field to autocomplete (`SRC_ADDR`, etc.) |
| `term` | autocomplete | Search prefix |

---

## Response envelope

Flow Functions wrap their content in the standard Function
envelope (same shape as topology and logs):

| Key | Description |
|---|---|
| `status` | HTTP-style status |
| `v` | Function schema version |
| `type` | **`flows`** -- the family discriminator |
| `help` / `accepted_params` / `required_params` / `has_history` / `update_every` | Discovery metadata |
| `data` | Mode-specific payload (object) |

### `data` object -- mode `flows`, view `table-sankey`

| Key | Description |
|---|---|
| `schema_version` | `2.0` |
| `source` | `netflow` |
| `layer` | `3` |
| `agent_id` | Producing-agent identifier |
| `collected_at` | RFC3339 timestamp |
| `view` | Echo of requested view |
| `group_by` | Echo of requested group-by tuple |
| `columns` | Per-column display metadata |
| `flows[]` | Aggregated flow rows (one row per group_by tuple) |
| `stats` | Counters: `flows_total`, `packets_total`, `bytes_total`, etc. |
| `metrics` | Optional metric block |
| `warnings[]` | Optional non-fatal diagnostics |
| `facets` | When `facets` was requested in body, per-field value-counts plus `selections` echo |

### `data` object -- mode `flows`, view `timeseries`

Replaces `flows[]` with `metric` (string) and `chart` (object); used
for line/area charts of bytes-per-second / packets-per-second
broken down by the group-by tuple.

### `data` object -- mode `flows`, geo views (`country-map`, `state-map`, `city-map`)

Returns geo-keyed aggregations (per-country / per-state / per-city
totals) suitable for map rendering.

### `data` object -- mode `autocomplete`

| Key | Description |
|---|---|
| `mode` | `autocomplete` |
| `field` | Echo of requested field |
| `term` | Echo of requested search term |
| `values[]` | Matching values for the field |
| `stats` / `warnings` | Same as flows mode |

---

## Examples

### Example 1: top-100 talker pairs by bytes, last hour

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "mode":     "flows",
  "view":     "table-sankey",
  "after":    -3600,
  "before":   0,
  "group_by": ["SRC_ADDR", "DST_ADDR"],
  "sort_by":  "bytes",
  "top_n":    100
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=flows:netflow" \
  -d "$PAYLOAD" \
  | jq '.data.flows[:5]'
```

### Example 2: breakdown of TCP traffic by AS name, with histogram

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "mode":       "flows",
  "view":       "timeseries",
  "after":      -86400,
  "before":     0,
  "selections": { "PROTOCOL": ["TCP"] },
  "group_by":   ["DST_AS_NAME"],
  "sort_by":    "bytes",
  "top_n":      25
}
EOF
```

### Example 3: country-map of egress bytes

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "mode":     "flows",
  "view":     "country-map",
  "after":    -3600,
  "before":   0,
  "group_by": ["DST_COUNTRY"],
  "sort_by":  "bytes",
  "top_n":    500
}
EOF
```

### Example 4: autocomplete for a destination IP filter

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "mode":  "autocomplete",
  "field": "DST_ADDR",
  "term":  "10.0.0."
}
EOF
```

### Example 5: discover the live parameter set first

```bash
read -r -d '' PAYLOAD <<'EOF'
{ "info": true }
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=flows:netflow" \
  -d "$PAYLOAD" \
  | jq '{accepted_params, required_params}'
```

---

## Limits and gotchas

- **Cloud timeout default 120 s.** Wide-window queries
  (`after: -86400`) over high-volume agents can hit it. Narrow
  the time window or filter via `selections`.
- **`top_n` is enumerated, not free.** Allowed values are 25, 50,
  100, 200, 500. Other integers are rejected.
- **`group_by` accepts up to 10 fields.** The order matters --
  it's the tuple ordering for the aggregation key.
- **AS names depend on the configured GeoIP/AS database.** If the
  collector has no AS database, `SRC_AS_NAME` / `DST_AS_NAME`
  will be empty strings. Same for country/city fields.
- **Privacy**: flow records reveal who-talks-to-whom and how much.
  Treat raw output as production-sensitive; never paste into
  committed files. Direct working output to
  `<repo>/.local/audits/...` (gitignored).
- **Sampled vs full flows**: NetFlow v5/v9 and sFlow are sampled
  by source devices; reported byte/packet counts are scaled by
  the sample rate. The collector reports raw counts -- consult
  source-device sampling configuration when interpreting
  absolute volumes.
- **Function is L3-only.** No L2 visibility (use
  [topology Functions](./query-topology.md) for L2). No
  application-layer dissection (use logs).
