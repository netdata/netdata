# Compare explicit node scope with all room nodes

## Question

What changes when a Cloud Scope Data query contains an explicit list of node
UUIDs instead of selecting all current nodes in the room with `"*"`?

## Inputs

- `SPACE`: Space UUID.
- `ROOM`: Room UUID.
- `CONTEXT`: metric context to query, such as `system.cpu`.
- A repository `.env` containing `NETDATA_CLOUD_TOKEN` and
  `NETDATA_CLOUD_HOSTNAME`.

Prepare the token-safe wrapper and a private audit directory once:

```bash
cd /path/to/netdata

source docs/netdata-ai/skills/query-netdata-agents/scripts/_lib.sh
agents_load_env

SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"
CONTEXT="system.cpu"
BEFORE=$(date +%s)
AFTER=$((BEFORE - 300))

AUDIT="$(agents_audit_dir)/cloud-node-scope-comparison"
mkdir -p "$AUDIT"
chmod 0700 "$AUDIT"
umask 077
```

Raw node inventories and query responses contain node identities. Keep them
under `.local/audits/`; do not paste them into issues, documentation, or logs.

## Steps

### 1. Capture the current room node inventory

This provides the explicit snapshot used by the comparison.

```bash
agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" \
  '{}' >"$AUDIT/room-nodes.json"

jq '[.nodes[].nd]' "$AUDIT/room-nodes.json" \
  >"$AUDIT/explicit-node-ids.json"
```

This step makes one wrapper call. The `jq` command only prepares the private
input for the next call.

### 2. Query the explicit node snapshot efficiently

Put UUIDs only in `scope.nodes`. Use `"*"` in `selectors.nodes` to select every
node already admitted by that scope.

```bash
jq -n \
  --slurpfile nodes "$AUDIT/explicit-node-ids.json" \
  --arg context "$CONTEXT" \
  --argjson after "$AFTER" \
  --argjson before "$BEFORE" \
  '{
    scope: {
      nodes: $nodes[0],
      contexts: [$context]
    },
    selectors: {
      nodes: ["*"],
      contexts: ["*"],
      instances: ["*"],
      dimensions: ["*"],
      labels: ["*"],
      alerts: ["*"]
    },
    window: {
      after: $after,
      before: $before,
      points: 1
    },
    aggregations: {
      metrics: [{group_by: ["node"], aggregation: "sum"}],
      time: {time_group: "average"}
    },
    format: "json2",
    options: ["jsonwrap", "minify", "unaligned"],
    timeout: 180000
  }' >"$AUDIT/request-explicit-scope.json"

agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  "@$AUDIT/request-explicit-scope.json" \
  >"$AUDIT/response-explicit-scope.json"
```

Passing `@file` through `agents_query_cloud` is important for large fleets. A
large JSON body passed inline becomes one process argument and can exceed the
operating system's per-argument limit before cURL makes a network request.

### 3. Query all current room nodes

Omit `scope.nodes`. Keep `selectors.nodes: ["*"]`.

```bash
jq -n \
  --arg context "$CONTEXT" \
  --argjson after "$AFTER" \
  --argjson before "$BEFORE" \
  '{
    scope: {
      contexts: [$context]
    },
    selectors: {
      nodes: ["*"],
      contexts: ["*"],
      instances: ["*"],
      dimensions: ["*"],
      labels: ["*"],
      alerts: ["*"]
    },
    window: {
      after: $after,
      before: $before,
      points: 1
    },
    aggregations: {
      metrics: [{group_by: ["node"], aggregation: "sum"}],
      time: {time_group: "average"}
    },
    format: "json2",
    options: ["jsonwrap", "minify", "unaligned"],
    timeout: 180000
  }' >"$AUDIT/request-room-scope.json"

agents_query_cloud POST \
  "/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  "@$AUDIT/request-room-scope.json" \
  >"$AUDIT/response-room-scope.json"
```

### 4. Compare sanitized response summaries locally

This step makes no API call and does not print node identities:

```bash
for response in \
  "$AUDIT/response-explicit-scope.json" \
  "$AUDIT/response-room-scope.json"; do
  jq '{
    summary_nodes: ((.summary.nodes // []) | length),
    selected_nodes: (.totals.nodes.sl // 0),
    queried_nodes: (.totals.nodes.qr // 0),
    result_series: (((.result.labels // []) | length) - 1),
    result_rows: ((.result.data // []) | length),
    timings: .timings
  }' "$response"

  jq -cS '.result' "$response" | sha256sum
done
```

For a stable room whose explicit snapshot still equals current membership, the
two forms target the same node universe and should produce equivalent results.
Sequential calls can still differ if routing, context availability, or node data
changes between them.

This equivalence applies only to the two request forms shown above: explicit
`scope.nodes` plus a wildcard selector, and omitted `scope.nodes` plus a wildcard
selector. It does not apply to duplicating a fleet-sized UUID list in
`selectors.nodes`; that form can be corrupted in downstream transport.

## Output

Return only:

- explicit snapshot node count;
- current room node count;
- whether the two node-ID sets match;
- selected and successfully queried node counts for each response;
- response/result equality or hashes;
- wall-clock and response timing summaries;
- any HTTP status or timeout, with identifiers redacted.

Do not return raw response bodies, UUIDs, hostnames, machine GUIDs, claim IDs, or
tokens.

## Verified oversized-selector failure decomposition

In one sanitized large-fleet A/B, both requests retained the same explicit
`scope.nodes` list and every other field. Only `selectors.nodes` changed between
`["*"]` and a duplicate copy of the full UUID list.

| Response section | `"*"` selector | UUID-list selector | Delta |
|---|---:|---:|---:|
| Entire response | 3,113,406 bytes | 42,218,154 bytes | +39,104,748 bytes |
| `summary` | 2,084,959 bytes | 35,741,434 bytes | +33,656,475 bytes |
| `result` | 225,791 bytes | 6,240,138 bytes | +6,014,347 bytes |
| `view` | 439,166 bytes | 145,786 bytes | -293,380 bytes |
| `db` | 362,273 bytes | 89,515 bytes | -272,758 bytes |

The UUID-list response was larger even though it returned fewer nodes and
series:

- `summary.instances` grew from 4,463 entries / 832,077 bytes to 399,230
  entries / 35,248,814 bytes. This single field added 34,416,737 bytes, or
  88.01% of the total response delta.
- Time-series rows grew from 1 to 301. Result value cells grew from 4,437 to
  441,868, while returned series fell from 4,437 to 1,468.
- Node metadata fell from 4,463 to 1,620 entries, offsetting 824,911 bytes of
  the expansion. The percentages of positive contributors therefore exceed
  100% before subtracting the smaller sections.

This is evidence of a changed query, not legitimate UUID-list overhead. The
oversized `nodes` selector is truncated before `points`, `scope_contexts`, and
`scope_nodes`. The missing context scope expands metadata to excluded contexts
and instances; the missing one-point request defaults to all available points;
and the missing node scope makes the truncated selector prefix act as scope.

Cloud split the scoped nodes across three routed requests in this run. Their
decoded Agent URLs were 52,207, 55,500, and 58,312 bytes with `"*"`, all below
the 65,536-byte limit. Duplicating the UUID list in the selector increased them
to 278,164, 281,457, and 284,269 bytes. Truncation occurred inside `nodes`
after approximately 1,767 complete UUID patterns, before any later parameter.

## Notes / gotchas

- **All current room nodes:** omit `scope.nodes`; set
  `selectors.nodes: ["*"]`.
- **Fixed subset with tight metadata:** put exact UUIDs in `scope.nodes`; keep
  `selectors.nodes: ["*"]` unless a second data-only filter is genuinely needed.
- **Do not use `scope.nodes: ["*"]`:** the Cloud JSON endpoint validates scope
  node entries as UUIDs and rejects `"*"` with HTTP 400.
- **Never duplicate a fleet-sized UUID list in `selectors.nodes`:** Cloud sends
  the full selector to every routed Agent or Parent in a GET query parameter.
  The Agent decodes at most 65,536 URL bytes and silently keeps the truncated
  prefix. Since the `nodes` parameter sorts before parameters including
  `options`, `points`, `scope_contexts`, `scope_nodes`, `time_group`, and
  `timeout`, truncation can discard those parameters too. Verified outcomes
  include a wrong node subset, a one-point request returning the default
  per-second rows, metadata expansion, a much larger response, and timeout.
  Keep exact UUIDs only in `scope.nodes` and use `selectors.nodes: ["*"]`.
- **Large routed scopes still need care:** Cloud partitions `scope.nodes` by
  route, while it copies `selectors.nodes` wholesale. Partitioning made the
  wildcard-selector form fit in the verified case, but a sufficiently large
  single routed scope can still approach the same Agent URL limit.
- **Separate four independent effects:**
  - Node scope and selectors determine which nodes are eligible.
  - `group_by: ["node"]` can return one result series per eligible node.
  - The aggregation-pass chain determines the numeric values of those results.
  - Duplicating UUIDs in the selector can corrupt the downstream request before
    matching begins.

  A large series count, an inflated aggregate value, and an expensive request
  require different diagnoses. They are not evidence that UUID selection itself
  changes aggregation math. Compare the same node set and aggregation chain
  before attributing a difference to the selector form.
- **Scope and selector are different:** scope controls data and metadata;
  selectors filter queried data inside that scope.
- **Room-wide is dynamic:** nodes added to the room become eligible without
  rebuilding the request. An explicit list is a snapshot and cannot include
  later additions.
- **Revisit a copied `limit`:** `limit` caps returned result dimensions, not the
  room scope. With `group_by: ["node"]`, a value copied from today's node count
  can truncate future node series. Omit it when no explicit result cap is
  intended.
- **`"*"` does not guarantee one series per room node:** the context, dimensions,
  time window, routing, retention, and node state still determine which nodes
  have queryable data.
- **Keep `scope.contexts`:** omitting it broadens metadata to every context in the
  room and can produce a very large response.
- **Latency is not deterministic:** compare semantics and payload size first;
  transient routing or Agent delays can dominate individual timing samples.

## Source guides

- [`query-metrics.md`](../query-metrics.md) — Scope Data request fields,
  scope/selector semantics, response structure, and query limits.
- [`query-nodes.md`](../query-nodes.md) — room node inventory and node metadata.
- [`SKILL.md`](../SKILL.md) — authentication, token-safe wrappers, and sensitive
  data handling.
