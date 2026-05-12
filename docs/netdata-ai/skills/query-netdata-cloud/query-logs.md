# Query log Functions via Netdata Cloud

This guide is part of the [`query-netdata-cloud`](./SKILL.md) skill.
Read the [SKILL.md prerequisites](./SKILL.md#prerequisites) first.
For the generic Function transport and the canonical protocol
reference, see [query-functions.md](./query-functions.md). The
authoritative protocol spec is
`<repo>/src/plugins.d/FUNCTION_UI_REFERENCE.md` (specifically the
"Log Explorer Format" section).

Log Functions are the **Log Explorer** class of Functions
(`has_history: true` in their `info` response). They return a
**time-windowed skim** of a larger log dataset, with **facets**
(per-field value counts) for drill-down and an optional
**histogram** (bucketed counts over time) for context.

Four log Functions exist today, each backed by a different log
source. Their request and response shapes follow the same standard
envelope, but the field set differs per source:

| Function | Source | Notes |
|---|---|---|
| `systemd-journal` | systemd journal namespaces (system, user, namespace-specific, remote-forwarded) | Linux nodes |
| `windows-events` | Windows event log channels | Windows nodes |
| `macos-logs` | macOS unified log store through Apple's native OSLog framework | macOS nodes |
| `otel-logs` | OpenTelemetry logs ingested by the agent | Any node with the OTEL log receiver enabled |

Confirm which are registered on a node via the
function-listing endpoint in
[query-functions.md](./query-functions.md). Field names below are
illustrative for `systemd-journal`; the same Function payload keys
(`after`, `before`, `last`, `query`, `facets`, `histogram`,
`__logs_sources`, ...) apply to the other log Functions -- only the **values
and column names** differ per source.

---

## Endpoint

`POST /api/v2/nodes/{nodeId}/function?function=systemd-journal`

Same shape as any other Function call. The body is the
`systemd-journal` Function's payload.

---

## Discover the Function's parameters

Always start with `info=true` to confirm the current schema -- the
Function's parameter set evolves across agent versions.

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{ "info": true }
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=systemd-journal" \
  -d "$PAYLOAD"
```

The `accepted_params` array in the response tells you which keys the
agent currently accepts.

---

## Body keys (current as of `STATUS_FILE_VERSION = 28`)

| Key | Type | Purpose |
|---|---|---|
| `info` | bool | Discovery only; do not combine with a real query |
| `after` | int | Lower time bound, in **seconds** (struct field `after_s`). Positive = absolute Unix seconds; negative = relative seconds from `before`. NOT ms, NOT µs |
| `before` | int | Upper time bound, in **seconds** (struct field `before_s`). Positive = absolute Unix seconds; negative = relative seconds from now (`0` = now) |
| `last` | int | Page size (rows). Default 200 |
| `direction` | string | `backward` (default; newest first) or `forward` |
| `anchor` | int | Per-row cursor for pagination, in **microseconds** (matches the row timestamp values, which are µs — a different unit from `after`/`before`) |
| `query` | string | Free-text search (FTS) across indexed fields |
| `facets` | string[] | Field names to group by (returns counts per value) |
| `histogram` | string | Field name to bucket-by-time |
| `selections` | object | **The only working way to filter by source or by field** in the JSON body. `{"__logs_sources":[...]}` selects sources; `{"FIELD":[...]}` filters a facet. See [Selecting log sources](#selecting-log-sources-info--selections) — passing `__logs_sources` as a top-level key is silently ignored |
| `if_modified_since` | int | Tail mode -- skip if no new data |
| `data_only` | bool | Skip metadata for a faster query |
| `sampling` | int | Cap on rows scanned when search would otherwise be huge |
| `slice` | bool | Native backend filter (faster, less flexible) |
| `delta` | bool | Incremental histogram updates |
| `tail` | bool | Append-mode (combine with `if_modified_since`) |

`info=true` returns the current authoritative list; rely on it, not
this table, when in doubt.

---

## Selecting log sources (`info` → `selections`)

This is the single most important mechanism for log queries, and it
works **identically for every log Function** (`systemd-journal`,
`windows-events`, `otel-logs`, and any other built on the shared
libnetdata logs-query library `LQS`). Only the **source id values**
differ per Function; the request shape is the same.

The agent parses the source selector from the POST body **only** inside
the `selections` object, as an **array**
(`<repo>/src/libnetdata/facets/logs_query_status.h:329-410`,
`lqs_request_parse_json_payload`). There is no top-level
`__logs_sources` key in the JSON parser, so:

```json
{ "__logs_sources": "Netdata/Daemon" }          // ❌ silently IGNORED → queries ALL sources
{ "selections": { "__logs_sources": ["Netdata/Daemon"] } }   // ✅ filters server-side
```

> The top-level / function-string form `source:<id>` only applies to
> the on-agent function-string transport. Over the Cloud REST API the
> body is JSON, so you MUST use `selections.__logs_sources` (an array).

### Step 1 — discover the valid source ids with `info`

The `info=true` response contains a `__logs_sources` widget. Each
option carries the `id` you pass, plus its size, entry count, and time
coverage (retention) — invaluable for picking the right source and
knowing how far back data exists:

```bash
curl ... -d '{"info":true}' \
 | jq -r '.. | objects | select(.id=="__logs_sources") | .options[]
          | "\(.id)\t\(.info)"'
# windows-events example output:
#   All-Of-Netdata    4 channels, 3.08MiB, covering 3d 19h, 1.94K entries, last 2026-06-26T04:30:44Z
#   Netdata/Daemon    1 channel, 1028KiB, covering 3d 19h, 789 entries, last 2026-06-26T04:28:37Z
#   Netdata/Health    ...
# systemd-journal ids look different, e.g. all, all-local-system-logs,
# all-local-namespaces, <namespace-name> — always read them from info.
```

`__logs_sources` is a **multiselect**: pass one or more ids in the
array; selecting any source resets the default (which is "all
sources") so you get exactly the union you asked for
(`logs_query_status.h:407-410`).

### Step 2 — query with the source filter

```bash
read -r -d '' PAYLOAD <<'EOF'
{
  "after": -3600, "before": 0, "last": 200, "data_only": true,
  "selections": { "__logs_sources": ["Netdata/Daemon"] }
}
EOF
curl -sS -X POST -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=windows-events" \
  -d "$PAYLOAD"
```

### Why server-side source filtering matters (cost + robustness)

- It is **server-side**: the agent reads only the selected channels /
  namespaces. An unfiltered query reads *everything*, so on a host
  where one co-located source is noisy (e.g. a Windows box where
  `Microsoft-Windows-SystemDataArchiver`/SRUM emits ~10 events/sec),
  the result is dominated by noise — a 500-row page can span under a
  minute, and a wide unfiltered window can **time out or make the
  plugin exit before responding**.
- Combine with **`data_only:true`** to skip facet-count and histogram
  aggregation. That aggregation is the usual cause of timeouts on wide
  windows; `data_only` makes otherwise-failing historical queries
  succeed.
- Net recipe for reliable historical pulls: **filter the source via
  `selections`** + **`data_only:true`** + a bounded `last`. Then a
  single query can cover a long window cheaply.

---

## Response shape

The response uses the **standard Function envelope** (top-level
keys `status`, `v`, `type`, `help`, `accepted_params`,
`required_params`, `has_history`, `update_every`, `data`, ...).
For log Functions, `type` is the source name (`logs` family
discriminator). Verified live against the agent-events node:

| Top-level key | Description |
|---|---|
| `status` | HTTP-style status integer (200 on success) |
| `v` | Function schema version |
| `type` | Family discriminator (carries `logs`-family value) |
| `help` / `accepted_params` / `required_params` | Discovery metadata (see [query-functions.md](./query-functions.md#info-true-discovery)) |
| `data` | **Array** of row arrays -- this is the result rows |
| `columns` | Object keyed by column name; per-column metadata: `index` (position in each row of `data`), `name` (display label), `type` (string / timestamp / integer / ...), `visible`, `unique_key`, `sort`, `summary` (`count` / `min` / `max` / `sum` / ...), `filter` (e.g. `range`), `visualization`, `value_options` (for transforms like `datetime_usec`) |
| `facets` | Array of facet records: `{id, name, options[]}` where each option is `{id, name, count}`. Use to drill down by field value. |
| `histogram` | If requested: time-bucketed counts. Object with `chart`, `id`, `name`, plus per-bucket data |
| `pagination` | Cursor info (`anchor`, `direction`, `last`, ...) for the next page |
| `default_charts` | Suggested chart configuration |
| `default_sort_column` | Recommended sort column |
| `available_histograms` | Field names that the agent can histogram-bucket |
| `_request` | Echo of the parsed request (defaults applied) |
| `versions` | Source/version map for cache invalidation |
| `last_modified` | Last-data timestamp |
| `expires` | Suggested cache expiry |
| `partial` | True if the result was capped by `sampling` or timeout |
| `message` | Optional info / warning string |
| `_journal_files` / `_fstat_caching` / `_sampling` / `_stats` | systemd-journal-specific debug counters |

### Reading rows

`data` is an array of rows. Each row is itself an array whose
positions match `columns.<key>.index`. To pretty-print a single
row by column name:

```bash
jq '.columns as $c
   | .data[0] as $row
   | $c | to_entries
        | sort_by(.value.index)
        | map({(.key): $row[.value.index]})
        | add' response.json
```

---

## Multi-value field selections (AND-of-OR filtering)

The `selections` POST-payload key is a structured field-filter
mechanism the Netdata `systemd-journal` Function (powered by the
libnetdata `facets` engine) supports. It is **distinct from raw
journalctl's `KEY=value` matches**: a single field can carry
multiple allowed values, and multiple fields are AND'd.

### Shape

`selections` is an object whose keys are journal field names
and whose values are arrays of allowed values:

```json
{
  "selections": {
    "FIELD1": ["A", "B", "C"],
    "FIELD2": ["D", "E"]
  }
}
```

Semantics (verified at
`<repo>/src/libnetdata/facets/logs_query_status.h:386-466`):

- **Between fields: AND.** All listed fields must match.
- **Between values for the same field: OR.** Any one of the
  listed values matches.

So the example above is logically:

```
(FIELD1 in A, B, C) AND (FIELD2 in D, E)
```

### Why this matters for performance

A namespace can hold tens of thousands to hundreds of thousands
of records per day. A bare `query` (FTS) scans every record's
indexed text fields. Structured `selections` matches use the
facet engine's per-field index, which is dramatically faster
once the time window is fixed.

**Rule of thumb:** narrow with `selections` first, then refine
with `query` (FTS) only as a residual narrower over the
already-sliced subset.

### Reserved keys inside `selections`

- `__logs_sources` (per `LQS_PARAMETER_SOURCE`,
  `logs_query_status.h:407`) is the source-type filter — pass the
  source ids from `info` as an array. This is the **only** way to
  scope sources in the JSON body; a top-level `__logs_sources` key is
  not parsed. See [Selecting log sources](#selecting-log-sources-info--selections).
- `query` inside `selections` is ignored
  (`logs_query_status.h:398`); use the top-level `query`.

### Example: structured filter + FTS narrower

```json
{
  "after":  -86400,
  "before": 0,
  "last":   500,
  "selections": {
    "__logs_sources":   ["agent-events"],
    "AE_AGENT_HEALTH":  ["crash-first", "crash-loop", "crash-repeated", "crash-entered"],
    "AE_AGENT_VERSION": ["v2.10.0", "v2.10.0-135-nightly"]
  },
  "query": "deadlock"
}
```

This selects the cross-product of crash-class records on those
two versions (index-resolved), then FTS-filters the result for
the substring `deadlock`. Index-friendly even on a
~200k-records-per-day namespace.

### Anti-pattern (avoid)

```json
{
  "after":  -604800,
  "before": 0,
  "selections": { "__logs_sources": ["agent-events"] },
  "query": "SIGSEGV"
}
```

A 7-day FTS over the entire namespace with no structured
narrowing. Slow and costly on large namespaces. Always pair FTS
with at least one structured `selections` field.

---

## Examples

### Example 1: most recent 50 entries from a specific namespace

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
NAMESPACE="systemd"   # or "agent-events", "any-namespace-name"

read -r -d '' PAYLOAD <<EOF
{
  "after":  -3600,
  "before": 0,
  "last":   50,
  "direction": "backward",
  "selections": { "__logs_sources": ["${NAMESPACE}"] }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=systemd-journal" \
  -d "$PAYLOAD"
```

### Example 2: full-text search with histogram

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"

read -r -d '' PAYLOAD <<'EOF'
{
  "after":  -86400,
  "before": 0,
  "last":   100,
  "query":  "OOM",
  "histogram": "PRIORITY",
  "facets":    ["_SYSTEMD_UNIT", "PRIORITY"],
  "selections": { "__logs_sources": ["all-local-system-logs"] }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=systemd-journal" \
  -d "$PAYLOAD"
```

### Example 3: paginate forward from a known anchor

```bash
TOKEN="YOUR_API_TOKEN"
NODE="YOUR_NODE_UUID"
ANCHOR=1700000123456789   # cursor from previous response

read -r -d '' PAYLOAD <<EOF
{
  "anchor":    ${ANCHOR},
  "direction": "forward",
  "last":      200,
  "selections": { "__logs_sources": ["all-local-logs"] }
}
EOF

curl -sS -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v2/nodes/$NODE/function?function=systemd-journal" \
  -d "$PAYLOAD"
```

---

## Limits and gotchas

- **`after`/`before` are in SECONDS** (struct fields `after_s` /
  `before_s`, `logs_query_status.h:345-346`): positive = absolute Unix
  seconds, negative = relative seconds (`before:-3600` = "one hour
  ago"; `before:0` = now). **`anchor` and the row timestamps are in
  microseconds** — do not reuse a row timestamp as a positive
  `after`/`before`. Mixing the two units is the most common bug.
- **Default cloud timeout is 120 s.** Wide windows that compute facets
  or a histogram are the usual offenders. Add **`data_only:true`** to
  skip that aggregation, **filter the source via `selections`**, and/or
  use `sampling` — see [Selecting log sources](#selecting-log-sources-info--selections).
  If a query still "exits before responding", narrow the window further.
- **Always scope the source via `selections.__logs_sources`.** With no
  source filter the query targets every source, which on a busy host
  can be hundreds of GB (Linux) or dominated by a noisy channel
  (Windows) — slow, and prone to timeouts.
- **Permission**: the cloud token must have a role that includes
  log-read access (function tags include `logs`). `scope:all` works;
  `scope:grafana-plugin` does NOT.
- **Response can be tens of MB** when `facets` include high-cardinality
  fields (`MESSAGE_ID`, `_BOOT_ID`, `_PID`). Pick facets carefully.

---

## Discovering a journal namespace

For `systemd-journal`, if the host runs `journalctl --namespace=<name>`,
the same `<name>` is a valid source id you pass in
`selections.__logs_sources`. As always, the authoritative list (with
per-source size, entry count, and retention coverage) comes from the
`info=true` response's `__logs_sources` widget `options[]` — see
[Selecting log sources](#selecting-log-sources-info--selections) for
the exact `jq` and the request shape.
