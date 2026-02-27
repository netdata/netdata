# Query time-series metrics from Netdata Cloud via the REST API.

## Mandatory Requirements (READ FIRST)

1. You MUST provide detailed and actionable instructions. You don't execute queries for users. You role is to educate them.

2. **Never ask users for credentials.** Do not request API tokens, Space IDs, or Room IDs. Always provide ready-to-use instructions with clear placeholders (`YOUR_API_TOKEN`, `YOUR_SPACE_ID`, `YOUR_ROOM_ID`) so users can substitute their own values locally. Your job is to teach users how to construct queries, not to execute queries on their behalf.

3. **`scope.contexts` MUST always be set.** Without it, the default scope is the entire room — every context, every instance, every dimension, every label across all nodes. This causes a **metadata explosion**: the response will contain megabytes of metadata for thousands of metrics the user didn't ask about. Always set `scope.contexts` to the specific context(s) relevant to the query (e.g., `["system.cpu"]`, `["disk.space"]`).

4. **Every response MUST include a complete, runnable curl command.** Users come here to get a query they can run — not a description of what a query would look like. If your response does not contain a full curl command with the complete JSON request body, you have failed to help the user. Specifically:
   - Always include the full `curl -X POST` command with headers, URL, and the entire `-d '{...}'` JSON body.
   - The JSON body must include all required fields: `scope`, `selectors`, `window`, `aggregations`, `format`, `options`, and `timeout`.
   - Set the 3 credentials as variables at the top: `TOKEN="YOUR_API_TOKEN"`, `SPACE="YOUR_SPACE_ID"`, `ROOM="YOUR_ROOM_ID"`.
   - Use a heredoc for the JSON payload (`read -r -d '' PAYLOAD <<'EOF' ... EOF`) so no escaping is needed.
   - The user must be able to copy your command, replace the 3 variables at the top, and run it immediately in their terminal.
   - A response that describes parameters or explains concepts without providing the actual runnable command is incomplete and unhelpful.
   - Even for simple questions, always provide the curl command. When in doubt, show the command.

---

## Prerequisites

Three things are needed:

### 1. API Token

1. Login to [app.netdata.cloud](https://app.netdata.cloud)
2. Click user icon (lower-left corner, tooltip shows your name)
3. Select **User Settings**
4. In the modal, select the **API Tokens** tab
5. Click the **[+]** button (top-left)
6. Select a scope, enter a description, click **Create**
7. **Copy the token immediately** — it will not be shown again

Relevant scopes: `scope:all` (full access), `scope:grafana-plugin` (data endpoints).

### 2. Space ID

1. In the dashboard left side, at the spaces list, click the **gear icon** below the spaces list (tooltip: "Space Settings")
2. In the **Info** tab, copy the **Space Id**

### 3. Room ID

1. In the same Space Settings, go to the **Rooms** tab
2. Find the room, click the **>** icon at the right of the room row (tooltip: "Room Settings")
3. In the **Room** tab, copy the **Room Id**

---

## API Endpoints

Base URL: `https://app.netdata.cloud`
Swagger online: https://app.netdata.cloud/api/docs/

All endpoints use **POST** with a JSON body and require:

```
Authorization: Bearer YOUR_API_TOKEN
Content-Type: application/json
```

| Endpoint | Purpose |
|----------|---------|
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/data` | Query time-series data |
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/nodes` | List nodes in the room |
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/contexts` | List available metric contexts |

---

## Discover Nodes

**Endpoint:** POST `/api/v3/spaces/{spaceID}/rooms/{roomID}/nodes`
**Body:** `{}`

Response fields per node:

| JSON field | Description |
|------------|-------------|
| `nd` | **Node UUID** — required for `scope.nodes` in data queries |
| `nm` | Hostname |
| `mg` | Machine GUID |
| `state` | `reachable` (live) or `stale` (disconnected) |
| `v` | Agent version |
| `labels` | All node labels as key-value pairs |
| `hw` | Hardware: `cpus`, `memory`, `disk_space`, `architecture` |
| `os` | OS: `nm` (name), `v` (version), `kernel` |
| `health` | Alert summary: `status`, `alerts.warning`, `alerts.critical` |
| `capabilities` | Supported features: `ml`, `funcs`, `health`, etc. |

Example:

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/nodes" \
  -d '{}'
```

---

## Discover Contexts

Contexts are metric types (e.g., `system.cpu`, `disk.space`, `net.net`).

**Endpoint:** POST `/api/v3/spaces/{spaceID}/rooms/{roomID}/contexts`

```json
{
  "scope": { "contexts": ["system.*"] },
  "selectors": { "nodes": ["*"], "contexts": ["*"] }
}
```

`scope.contexts` supports patterns: `system.*`, `disk.*`, `*cpu*`.

---

## Query Metric Data

**Endpoint:** POST `/api/v3/spaces/{spaceID}/rooms/{roomID}/data`

### Full Request Body Structure

```json
{
  "scope": {
    "nodes": [],
    "contexts": ["REQUIRED — e.g. system.cpu, disk.space"],
    "instances": [],
    "dimensions": [],
    "labels": []
  },
  "selectors": {
    "nodes": ["*"],
    "contexts": ["*"],
    "instances": ["*"],
    "dimensions": ["*"],
    "labels": ["*"],
    "alerts": ["*"]
  },
  "window": {
    "after": 0,
    "before": 0,
    "points": 0,
    "duration": 0,
    "tier": null,
    "baseline": null
  },
  "aggregations": {
    "metrics": [
      {
        "group_by": [],
        "group_by_label": [],
        "aggregation": "avg"
      }
    ],
    "time": {
      "time_group": "average",
      "time_group_options": null,
      "time_resampling": null
    }
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 10000,
  "limit": null
}
```

---

### scope — Define the Data Universe

Scope controls **both data and metadata** in the response. Use scope fields for filtering so that the response metadata is focused on what you asked for.

**WARNING**: The default scope (when fields are omitted) is **all nodes and all contexts in the room**. This can produce multi-megabyte responses with metadata for thousands of metrics. `scope.contexts` MUST always be set to avoid this metadata explosion.

| Field | Type | Accepts | Default (if omitted) |
|-------|------|---------|---------------------|
| `nodes` | `string[]` | **Node UUIDs only** (the `nd` field from `/nodes`) | All nodes in the room |
| `contexts` | `string[]` | Exact names or patterns (`system.*`, `*cpu*`) | **REQUIRED** — always set to avoid metadata explosion |
| `instances` | `string[]` | Exact names or patterns (`disk_space./@NODE_UUID`) | All instances |
| `dimensions` | `string[]` | Exact names or patterns (`*user*`, `sent`) | All dimensions |
| `labels` | `string[]` | `key:value` pairs (`filesystem:btrfs`, `mount_point:/`) | No label filter |

Multiple entries in the same field are OR-combined. Multiple `labels` entries with different keys are AND-combined.

**Filtering by node**: Use `selectors.nodes` with hostname patterns (e.g., `["web*", "prod-*"]`). This is the simplest and preferred approach. Metadata will include all nodes in the room, but data will be filtered correctly.

**Advanced**: `scope.nodes` restricts both data AND metadata, but it only accepts node UUIDs (the `nd` field from `/nodes`). Hostnames, patterns, and wildcards do not work. Use this only when you need tight metadata scoping — otherwise prefer `selectors.nodes`.

CRITICAL: `scope.contexts` MUST always be set. The context is the metric type shown next to the chart title on the Netdata dashboard (e.g., `system.cpu`, `disk.space`). Clicking it copies it to the clipboard.

---

### selectors — Further Filter Data Within the Scope

Selectors filter **data only** — response metadata still reflects the full scope (the room). For programmatic API queries, use `scope` for filtering and set all selectors to `["*"]`.

Selectors exist for the Netdata dashboard, which needs full metadata to show context ("the whole") while displaying a filtered subset.

| Field | Type | Checked against | Supports |
|-------|------|----------------|----------|
| `nodes` | `string[]` | Machine GUID, node ID, **hostname** | Simple patterns, positive and negative |
| `contexts` | `string[]` | Context ID | Simple patterns, positive and negative |
| `instances` | `string[]` | Instance ID, instance name, `instance@machine_guid` | Simple patterns, positive and negative |
| `dimensions` | `string[]` | Dimension ID and dimension name | Simple patterns, positive and negative |
| `labels` | `string[]` | `name:value` of all labels | Simple patterns (negative not recommended) |
| `alerts` | `string[]` | Alert name, `name:status` (CLEAR, WARNING, CRITICAL, REMOVED, UNDEFINED, UNINITIALIZED) | Simple patterns; negative excludes instances |

**`selectors.nodes` is the preferred way to filter by node.** It accepts hostname patterns (e.g., `["web*", "!staging*"]`), making it simpler than looking up UUIDs for `scope.nodes`. Metadata will include all nodes in scope, but data is filtered correctly.

CRITICAL: `scope.contexts` MUST always be set to avoid metadata explosion.

---

### window — Time Range

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `after` | `int` | Start time. Negative = relative seconds from `before` (max -94608000 = 3 years). Positive = Unix epoch. | `-600` |
| `before` | `int` | End time. Negative = relative seconds from now (max -94608000). Positive = Unix epoch. | `0` (now) |
| `points` | `int` | Number of data points to return. `0` or omitted = all available points. | `0` |
| `duration` | `int` | Alternative to after/before. Duration in seconds. | `0` |
| `tier` | `int?` | Force a specific dbengine storage tier (0 = per-second, 1 = per-minute, 2 = per-hour). `null` = auto-select. | `null` |
| `baseline` | `object?` | Baseline window for comparison queries. Same fields as window: `after`, `before`, `points`, `duration`. | `null` |

Max points requested: approximately **500** (`ScopeDataRequestMaxPoints`). The Cloud clamps the request to 500 before forwarding to agents, but the actual number returned may vary slightly due to time alignment.

The time range is divided into `points` equal intervals. Each interval is aggregated using the `time_group` function.

---

### How the Query Pipeline Works

The query engine is a pipeline with two aggregation stages:

1. **Identify time-series** matching the `scope` and `selectors`
2. **Set up the output time-series** based on `group_by` (e.g., 2 groups for label values A and B)
3. **For each matched time-series:**
   - **Stage 1 — Time aggregation** (`time_group`): Aggregate raw samples within each time interval into `points` data points (e.g., average 86400 per-second samples into 100 points)
   - **Stage 2 — Metric aggregation** (`aggregation`): Add the time-aggregated points into the appropriate output time-series using the aggregation function (e.g., SUM into group A or B)
4. **Present** the grouped, aggregated result

**Key insight**: `time_group` reduces samples within each time-series. `aggregation` combines multiple time-series into groups. They operate in sequence — metric aggregation works on already time-aggregated data.

#### Example: 1000 containers, group by label `namespace` (2 values: A and B), 100 points over 1 day

1. Output setup: 2 time-series needed (A and B), each with 100 points
2. For each of the 1000 container time-series:
   - Time-aggregate 86400 seconds into 100 points using `time_group` (e.g., `average`)
   - Add those 100 points into either A or B using `aggregation` (e.g., `sum`)
3. Result: 2 columns (A, B) × 100 rows

#### Choosing time_group Based on What the User Wants

| User intent | time_group | Why |
|-------------|-----------|-----|
| Average resource consumption (rate metrics: CPU, I/O, bandwidth) | `average` | Rate metrics represent per-second rates; averaging preserves the rate |
| Average resource consumption (gauge metrics: memory, disk space, connections) | `average` or `max` | Gauges represent current state; max shows peak usage |
| Find spikes or peaks (any metric type) | `max` | Captures the highest value within each interval |
| Total volume transferred (counters: bytes, packets) | `sum` | Sums the actual volume |
| Count events matching a condition | `countif` | Counts samples matching a threshold |

#### Choosing aggregation Based on How to Combine Series

| User intent | aggregation | Why |
|-------------|------------|-----|
| Total across all series (e.g., total CPU across all containers) | `sum` | Adds up all contributions |
| Average across series | `avg` | Mean of the group |
| Worst case across series | `max` | Highest value in the group |
| Best case across series | `min` | Lowest value in the group |

#### Mapping User Questions to Parameters

**"Find a CPU spike over the last week across all my containers"**
→ `time_group: "max"`, `aggregation: "sum"` (sum user+system), `group_by: ["instance"]`

**"Which namespace consumed most CPU over the last week?"**
→ `time_group: "average"` (per-second rate) or `"sum"` (total), `aggregation: "sum"`, `group_by: ["label"]`, `group_by_label: ["namespace"]`

**"Peak memory usage per node over the last 24 hours"**
→ `time_group: "max"` (gauge metric, want peak), `aggregation: "sum"`, `group_by: ["node"]`

#### Research the Context Before Answering

Before constructing a query for a user, you should understand the metric context they are asking about — its dimensions, labels, and whether it represents rates (`incremental`) or gauges (`absolute`). Search for the context name (e.g., `cgroup.cpu`, `disk.space`, `nginx.connections`) in the Netdata source code to find its `metadata.yaml`, which defines dimensions, units, chart type, and available labels. This ensures you choose the correct `time_group` and `aggregation` for their use case.

---

### aggregations.time — Time Aggregation

Controls how raw data points within each time interval are combined into one value per series.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `time_group` | `string` | Aggregation function (see table below) | `average` |
| `time_group_options` | `string?` | Additional parameter for the function | `null` |
| `time_resampling` | `int?` | Resample "per-second" values to "per-minute" (60) or "per-hour" (3600). Only works with `time_group=average`. | `null` |

#### time_group values

| Value | Aliases | Description |
|-------|---------|-------------|
| `average` | `avg` | Mean value **(default)** |
| `min` | | Minimum value |
| `max` | | Maximum value |
| `sum` | | Sum of values |
| `median` | | Median value |
| `stddev` | | Standard deviation |
| `cv` | | Coefficient of variation (stddev/mean) |
| `ses` | | Single exponential smoothing |
| `des` | | Double exponential smoothing |
| `incremental-sum` | | Difference between last and first value in interval |
| `countif` | | Count values matching condition. Set condition in `time_group_options`: `">0"`, `"=0"`, `"!=0"`, `"<=10"` |
| `percentile` | | Percentile. Set percentile value in `time_group_options`: `"95"`, `"99"` |
| `trimmed-mean` | | Mean after trimming outliers. Set trim % in `time_group_options` |
| `trimmed-median` | | Median after trimming outliers. Set trim % in `time_group_options` |

IMPORTANT: when specifying any time_group except `min`, `max`, `avg`, `sum`, you MUST specify tier=0 to ensure a non-aggregated tier is used.

#### time_group_options values

| Used with | Value format | Example |
|-----------|-------------|---------|
| `countif` | Comparison operator + value | `">0"`, `"=0"`, `"!=0"`, `"<=100"` |
| `percentile` | Percentile value (0-100) | `"95"`, `"99.5"` |
| `trimmed-mean` | Trim percentage | `"5"`, `"10"` |
| `trimmed-median` | Trim percentage | `"5"`, `"10"` |

IMPORTANT: when specifying any time_group except `min`, `max`, `avg`, `sum`, you MUST specify tier=0 to ensure a non-aggregated tier is used.

---

### aggregations.metrics[] — Dimension Aggregation

Controls how multiple time-series are combined. Each entry defines a grouping pass. At least one is required.

| Field | Type | Description | Default |
|-------|------|-------------|---------|
| `group_by` | `string[]` | What to group by (see table below) | (required) |
| `group_by_label` | `string[]` | Label keys to group by. Required when `group_by` includes `label`. Order is respected. | `[]` |
| `aggregation` | `string` | How to combine grouped values (see table below) | `average` |

#### group_by values

All values can be combined together **except** `selected` (if `selected` is present, all others are ignored).

| Value | Result columns represent | Use case |
|-------|------------------------|----------|
| `selected` | Single column: all matched data combined into one series | Total/aggregate value across everything |
| `dimension` | One column per unique dimension name | Break down by metric component (user/system/iowait for CPU) |
| `node` | One column per node | Compare nodes side by side |
| `instance` | One column per instance (`context@hostname`) | Compare instances across nodes |
| `label` | One column per unique label value | Group by label (requires `group_by_label`) |
| `context` | One column per context | Compare different metric types |
| `units` | One column per unit type | Group by measurement unit |
| `percentage-of-instance` | Percentages per dimension within each instance | Show proportions instead of absolutes |

Combination example: `"group_by": ["node", "dimension"]` creates one column per node+dimension combination.

#### aggregation values

| Value | Aliases | Description |
|-------|---------|-------------|
| `avg` | `average` | Mean of grouped values **(default)** |
| `sum` | | Sum of grouped values |
| `min` | | Minimum of grouped values |
| `max` | | Maximum of grouped values |
| `median` | | Median of grouped values |
| `percentage` | | Express as percentage of total |

---

### format

Only `json2` is supported by Netdata Cloud.

---

### options

Array of strings. Each option modifies the response behavior.

| Option | Description |
|--------|-------------|
| `jsonwrap` | **Recommended.** Wraps the result with metadata (summary, view, db, timings) |
| `minify` | **Recommended.** Minimizes JSON output size |
| `unaligned` | **Recommended for API queries.** Without this, time intervals are aligned to wall-clock boundaries based on the requested period (e.g., 1-hour queries snap to 00:00–01:00). This is useful for dashboards (prevents charts from "dancing" on refresh) but confusing for API users who expect data for the exact time range they requested. Always use `unaligned` for programmatic queries. |
| `nonzero` | Exclude dimensions that have only zero values |
| `null2zero` | Replace null values with zero |
| `abs` | Return the absolute value of all data |
| `absolute` | Same as `abs` |
| `display-absolute` | Display absolute values |
| `flip` | Flip the sign of values (multiply by -1) |
| `reversed` | Reverse the order of data points (oldest last) |
| `min2max` | Show the range (max - min) instead of the value |
| `percentage` | Convert values to percentages |
| `seconds` | Return timestamps as seconds |
| `ms` | Return timestamps as milliseconds |
| `milliseconds` | Same as `ms` |
| `match-ids` | Match dimensions by ID only (not name) |
| `match-names` | Match dimensions by name only (not ID) |
| `anomaly-bit` | Return anomaly rate instead of metric values |
| `natural-points` | Return natural data points (one per collection interval) |
| `virtual-points` | Return virtual (interpolated) data points |
| `objectrows` | Return data rows as objects instead of arrays |
| `google_json` | Format compatible with Google Charts |

Recommended minimum: `["jsonwrap", "minify", "unaligned"]`

---

### timeout

Query timeout in milliseconds. Default: `10000` (10 seconds). Set higher for queries spanning many nodes or long time ranges.

### limit

Optional integer. Limits the number of dimensions returned. Cannot be negative. Useful when querying high-cardinality contexts.

---

## Response Structure

With `jsonwrap` option, the response contains:

### Top-level fields

| Field | Description |
|-------|-------------|
| `api` | API version (integer) |
| `agents` | List of agents consulted |
| `versions` | Hash values to detect database changes |
| `summary` | Metadata about nodes, contexts, instances, dimensions, labels, alerts |
| `totals` | Counts of selected/excluded/queried items |
| `functions` | List of supported functions |
| `db` | Database info (tiers, retention, update frequency) |
| `view` | Presentation metadata (title, units, dimensions, time range) |
| `result` | **The actual time-series data** |
| `timings` | Query performance metrics |

### summary

Metadata determined by `scope`. Statistics within are influenced by `selectors`.

```
summary.nodes[]     — ni (index), mg (machine GUID), nd (node UUID), nm (hostname), st (status), sts (stats)
summary.contexts[]  — id, is (instances count), ds (dimensions count), al (alerts), sts (stats)
summary.instances[] — id, nm (name), ni (node index), ds (dimensions count), al (alerts), sts (stats)
summary.dimensions[] — id, nm (name), ds (count), pri (priority), sts (stats)
summary.labels[]    — id (label key), vl[] (label values with id and stats)
summary.alerts[]    — nm (name), cl (clear count), wr (warning count), cr (critical count)
```

Stats object (`sts`): `min`, `max`, `avg` (average), `arp` (anomaly rate %), `con` (contribution %).

ItemsCount fields: `sl` (selected), `ex` (excluded), `qr` (query success), `fl` (query fail).

### view

| Field | Description |
|-------|-------------|
| `title` | Chart title |
| `update_every` | Data collection interval (seconds) |
| `after` | Actual start timestamp of returned data |
| `before` | Actual end timestamp of returned data |
| `points` | Number of data points returned |
| `units` | Unit of measurement |
| `chart_type` | Default chart type (line, area, stacked) |
| `min` | Minimum value across all data |
| `max` | Maximum value across all data |
| `dimensions.grouped_by` | Array confirming the `group_by` used |
| `dimensions.ids` | Unique dimension IDs |
| `dimensions.names` | Human-readable dimension names (column headers) |
| `dimensions.units` | Units per dimension |
| `dimensions.priorities` | Display priority per dimension |
| `dimensions.aggregated` | Number of source metrics aggregated into each dimension |
| `dimensions.sts` | Stats arrays per dimension: `min[]`, `max[]`, `avg[]`, `arp[]`, `con[]` |

### result — The Time-Series Data

```json
{
  "labels": ["time", "host1", "host2"],
  "point": {"value": 0, "arp": 1, "pa": 2},
  "data": [
    [1700000060, [5.23, 0, 0], [3.15, 0, 0]],
    [1700000120, [4.87, 0, 0], [2.91, 0, 0]]
  ]
}
```

- `result.labels` — column names. First is always `"time"`. Rest match `view.dimensions.names`.
- `result.point` — maps positions within each value array: `{"value": 0, "arp": 1, "pa": 2}`
- `result.data` — array of rows: `[timestamp, [col1_values], [col2_values], ...]`

Each value array contains 3 elements:
- **Index 0 (`value`)**: The metric value
- **Index 1 (`arp`)**: Anomaly rate (0-100). Percentage of raw samples in this interval flagged as anomalous by ML
- **Index 2 (`pa`)**: Point annotations bitmap. Values can be combined (OR'd):

| Bit | Value | Meaning |
|-----|-------|---------|
| (none) | `0` | Normal data point — no issues |
| bit 0 | `1` | **Empty** — no data was collected for this interval |
| bit 1 | `2` | **Reset** — a counter reset/overflow was detected |
| bit 2 | `4` | **Partial** — not all expected sources contributed to this point (e.g., in group-by queries, some series had no data) |

Values combine: e.g., `5` = empty + partial, `6` = reset + partial.

### db

| Field | Description |
|-------|-------------|
| `tiers` | Number of database tiers |
| `update_every` | Maximum update interval across nodes |
| `first_entry` | Earliest data timestamp |
| `last_entry` | Latest data timestamp |
| `per_tier[]` | Per-tier info: `tier`, `queries`, `points`, `update_every`, `first_entry`, `last_entry` |
| `units` | Database units |
| `dimensions.ids` | Database dimension IDs |
| `dimensions.units` | Database dimension units |
| `dimensions.sts` | Database-level stats |

### timings

| Field | Description |
|-------|-------------|
| `total_ms` | Total query time |
| `routing_ms` | Time to route to agents |
| `prep_ms` | Preparation time (per agent) |
| `query_ms` | Query execution time (per agent) |
| `output_ms` | Output formatting time (per agent) |
| `node_max_ms` | Slowest node response time |
| `cloud_ms` | Cloud processing time |

---

## How Users Find Metric Names in the UI

1. **Context names** (for `scope.contexts`):
   - The context is shown next to the chart title (e.g., `system.cpu`, `disk.space`). You can click it to copy it.
   - Use the **Metrics** tab in the dashboard to browse all available contexts
   - Use the `/contexts` endpoint with `scope.contexts: ["pattern*"]`

2. **Dimension names** (for `scope.dimensions`):
   - Visible in the chart legend (e.g., `user`, `system`, `iowait` for CPU)
   - Query with `group_by: ["dimension"]` to see all dimension names in `view.dimensions.names`

3. **Node hostnames and UUIDs** (for `scope.nodes`):
   - The **Nodes** tab lists hostnames
   - Use `/nodes` endpoint to get UUIDs (the `nd` field)

4. **Labels** (for `scope.labels` and `group_by_label`):
   - Click the labels drop-down on a chart, to see all label keys and values
   - Labels like `mount_point`, `filesystem`, `interface` appear in the list
   - Query with `group_by: ["selected"]` and check `summary.labels` in the response to discover available label keys and values for a context

---

## Practical Examples

All examples use this pattern — users replace the 3 variables at the top:

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"
```

### Example 1: Total CPU Across All Nodes (Last 10 Minutes)

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["selected"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: Single column `selected` with total CPU % (sum of all dimensions across all nodes) at 5 time points.

### Example 2: CPU Breakdown by Dimension

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["dimension"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: One column per dimension (`user`, `system`, `iowait`, `irq`, `softirq`, `steal`, `guest`, `nice`). Values are summed across all nodes.

### Example 3: Compare CPU Per Node

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["node"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: One column per node hostname. Values are total CPU % per node. Column names in `view.dimensions.names`.

### Example 4: Peak CPU Per Node Over Last Hour

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -3600, "before": 0, "points": 6},
  "aggregations": {
    "metrics": [{"group_by": ["node"], "aggregation": "max"}],
    "time": {"time_group": "max"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: 6 points (10-min intervals). Each value is the **peak** CPU for that node in that interval.

### Example 5: Disk Space Grouped by Filesystem Type

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["disk.space"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["label"], "group_by_label": ["filesystem"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: One column per filesystem type (ext4, btrfs, tmpfs, etc.). Values are total disk space summed across all nodes.

### Example 6: Filter by Specific Nodes (UUIDs)

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"], "nodes": ["NODE_UUID_1", "NODE_UUID_2"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["node"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: Data and metadata scoped to only those 2 nodes. First call `/nodes` to get UUIDs (the `nd` field).

### Example 7: Filter by Labels

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["disk.space"], "labels": ["mount_point:/", "filesystem:ext4"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["selected"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: Only ext4 root mount points. Multiple labels with different keys are AND-combined.

### Example 8: Filter Specific Dimensions

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["system.cpu"], "dimensions": ["user", "system"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 5},
  "aggregations": {
    "metrics": [{"group_by": ["dimension"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Result: Only `user` and `system` CPU dimensions.

### Example 9: Discover Labels for a Context

```bash
TOKEN="YOUR_API_TOKEN"
SPACE="YOUR_SPACE_ID"
ROOM="YOUR_ROOM_ID"

read -r -d '' PAYLOAD <<'EOF'
{
  "scope": {"contexts": ["disk.space"]},
  "selectors": {"nodes": ["*"], "contexts": ["*"], "instances": ["*"], "dimensions": ["*"], "labels": ["*"], "alerts": ["*"]},
  "window": {"after": -600, "before": 0, "points": 1},
  "aggregations": {
    "metrics": [{"group_by": ["selected"], "aggregation": "sum"}],
    "time": {"time_group": "average"}
  },
  "format": "json2",
  "options": ["jsonwrap", "minify", "unaligned"],
  "timeout": 30000
}
EOF

curl -s -X POST \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $TOKEN" \
  "https://app.netdata.cloud/api/v3/spaces/$SPACE/rooms/$ROOM/data" \
  -d "$PAYLOAD"
```

Then inspect `summary.labels` in the response:

```json
"summary": {
  "labels": [
    {"id": "filesystem", "vl": [{"id": "ext4"}, {"id": "btrfs"}, {"id": "tmpfs"}]},
    {"id": "mount_point", "vl": [{"id": "/"}, {"id": "/boot"}, {"id": "/home"}]}
  ]
}
```

---

## Known Limitations

1. **`scope.contexts` MUST always be set** — without it, the response includes metadata for every metric in the room (hundreds of contexts, thousands of instances). This causes multi-megabyte responses.
2. **`scope.nodes` accepts only node UUIDs** — use `selectors.nodes` with hostname patterns instead (simpler). Only use `scope.nodes` when you need tight metadata scoping.
3. **Only `json2` format** is supported by the Cloud API. Other formats (csv, ssv, etc.) are not reliably supported through the Cloud proxy.
4. **Max ~500 data points** per query. The Cloud clamps requests to 500 before forwarding to agents; actual returned count may vary slightly due to time alignment.
5. **Default timeout is 10 seconds** (10000ms). Increase for large/slow queries.
6. **Stale nodes** appear in `/nodes` but return no data. Check `state` field.
7. **Always use `unaligned` option** for API queries — without it, time intervals snap to wall-clock boundaries, which is confusing for programmatic use.

---

> **REMINDER — Credentials**: Do not request or accept user credentials. Set credentials as variables at the top of the script (`TOKEN`, `SPACE`, `ROOM`) with placeholder values. Users replace these 3 variables and run the command themselves.

> **REMINDER — Always show a runnable curl command**: Your response is only useful if it contains a complete, runnable script: 3 variables at the top, a heredoc `PAYLOAD` with clean JSON (no escaping), and the curl command. Never describe a query without showing it. Never summarize parameters without building the actual request. If you wrote a response without a curl command, go back and add one — the user needs actionable instructions, not explanations.

> **REMINDER — scope.contexts and unaligned**: Every query MUST set `scope.contexts` — omitting it returns metadata for the entire room (megabytes of irrelevant data). Every query MUST include `"unaligned"` in options — without it, time intervals snap to wall-clock boundaries instead of the requested time range.
