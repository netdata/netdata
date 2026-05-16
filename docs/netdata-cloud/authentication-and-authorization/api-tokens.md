# API Tokens

API tokens (Bearer tokens) enable you to access Netdata resources programmatically. These tokens authenticate and authorize API requests, allowing you to interact with Netdata services securely from external applications, scripts, or integrations.

:::important

API tokens never expire but should be managed carefully as they grant access to your Netdata resources.

:::

## Token Generation

**Location**

You can access token management through the Netdata UI:

1. Click your profile picture in the bottom-left corner
2. Select "User Settings"
3. Navigate to the API Tokens section

**Available Scopes**

You can limit each token to specific scopes that define its access permissions:

| Scope       | Description                                                                                                                              | API Access        |
| :---------- | :--------------------------------------------------------------------------------------------------------------------------------------- | :---------------- |
| `scope:all` | Grants the same permissions as the user who created the token. The token can access all spaces, rooms, and nodes the user has access to. | All API endpoints |

## Common Endpoints

With appropriate API tokens, you can access endpoints including:

- `/api/v2/spaces` - Space information
- `/api/v2/nodes` - Node information
- `/api/v2/data` - Multi-dimensional data queries
- `/api/v2/contexts` - Context metadata
- `/api/v2/weights` - Metric scoring/correlation
- `/api/v2/q` - Full-text search
- `/api/v1/info` - Agent information
- `/api/v1/charts` - Chart information
- `/api/v1/data` - Single node data queries

:::info

Currently, Netdata Cloud is not exposing the stable API.

:::

## Cloud API Space/Room Composite Endpoints

Netdata Cloud provides composite endpoints that operate on multiple nodes within a space or room. These endpoints support advanced aggregation and grouping capabilities for querying metric data across multiple nodes simultaneously.

### Endpoint Structure

```
POST /api/v1/spaces/{spaceID}/rooms/{roomID}/data
```

This endpoint queries aggregated metric data across all nodes in a specific room within a space.

### Request Body

The POST request accepts a JSON body with the following structure:

```json
{
  "aggregations": {
    "metrics": [
      {
        "aggregation": "sum",
        "group_by": ["dimension", "node"]
      }
    ],
    "time": {
      "time_group": "average",
      "time_group_options": null,
      "time_resampling": 60
    }
  },
  "format": "json2",
  "scope": {
    "contexts": ["system.cpu"],
    "nodes": ["node-id-1", "node-id-2"]
  }
}
```

### Aggregation Methods

The `aggregation` field supports the following methods:

- `avg` - Average value across grouped elements
- `min` - Minimum value across grouped elements
- `max` - Maximum value across grouped elements
- `sum` - Sum of values across grouped elements

### Grouping Options

The `group_by` field supports the following options:

- `dimension` - Group by metric dimension
- `node` - Group by node
- `chart` - Group by chart
- `label` - Group by label

### Time Grouping

The `time_group` field supports the following methods:

- `average` - Average over time interval
- `min` - Minimum over time interval
- `max` - Maximum over time interval
- `sum` - Sum over time interval
- `incremental-sum` - Incremental sum over time interval
- `median` - Median over time interval
- `trimmed-mean` - Trimmed mean over time interval
- `trimmed-median` - Trimmed median over time interval
- `percentile` - Percentile over time interval
- `stddev` - Standard deviation over time interval
- `coefficient-of-variation` - Coefficient of variation over time interval
- `ema` - Exponential moving average over time interval
- `des` - Double exponential smoothing over time interval
- `countif` - Count values matching a condition
- `extremes` - Max for positive, min for negative values

### Example Usage

**Query aggregated CPU data across multiple nodes in a room:**

```console
curl -X 'POST' \
  'https://app.netdata.cloud/api/v1/spaces/87b8-46b9-b34f-957a28e70d6e/rooms/c50be25c-5e48-457c-9f84-5afdc/data' \
  -H 'accept: application/json' \
  -H 'Authorization: Bearer <token>' \
  -H 'Content-Type: application/json' \
  -d '{
  "aggregations": {
    "metrics": [
      {
        "aggregation": "sum",
        "group_by": ["dimension"]
      }
    ],
    "time": {
      "time_group": "average",
      "time_resampling": 60
    }
  },
  "format": "json2",
  "scope": {
    "contexts": ["system.cpu"],
    "nodes": ["c37aed6a-aab7-423b-9d18-856f4", "94b7d15d-9a6e-4d1a-a26c-a5bd5"]
  }
}'
```

### API Version Mapping

Netdata uses different API versions for different purposes:

| API Type  | Version | Endpoint Pattern         | Use Case                                    |
| :-------- | :------ | :----------------------- | :------------------------------------------ |
| Agent API | v1, v2  | `/api/v1/*`, `/api/v2/*` | Deprecated, use v3                          |
| Agent API | v3      | `/api/v3/*`              | Stable, recommended for agent queries       |
| Cloud API | v1      | `/api/v1/spaces/*`       | Space/room composite queries                |
| Cloud API | v2      | `/api/v2/*`              | Node listing, space listing, simple queries |

**Key differences:**

- Agent API v3 is the stable version for querying individual agent metrics
- Cloud API uses v1 for space/room composite endpoints (not v3)
- Cloud API uses v2 for listing resources (spaces, nodes, rooms)
- Cloud API space/room endpoints support POST with JSON body for complex aggregations

## Example Usage

**Get the Netdata Cloud space list**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/spaces
```

**Get node information**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/nodes
```

**Query metric data**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/data?contexts=system.cpu&after=-600
```

**Advanced Metric Queries with Aggregation**

For more advanced queries with aggregation, use the POST endpoint with a JSON body. This allows you to query metrics with time aggregation (like average values) and control grouping and filtering.

```console
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

**Time Aggregation Options**

The `time_group` parameter in `aggregations.time` controls how data points within each time interval are combined:

| Option | Description | Use Case |
|--------|-------------|----------|
| `average` | Mean value (default) | Average resource consumption over time |
| `min` | Minimum value | Find lowest values in each interval |
| `max` | Maximum value | Find spikes or peaks |
| `sum` | Sum of values | Total volume transferred (counters) |
| `median` | Median value | Robust central tendency |
| `stddev` | Standard deviation | Measure of variability |
| `ses` | Single exponential smoothing | Trend-aware smoothing |
| `des` | Double exponential smoothing | Trend + seasonality smoothing |
| `incremental-sum` | Difference between last and first value | Change over interval |
| `percentile` | Generic percentile (set value in `time_group_options`) | e.g., 95th percentile latency |
| `countif` | Count values matching condition (set condition in `time_group_options`) | e.g., count samples above threshold |
| `trimmed-mean` | Mean after trimming outliers (set trim % in `time_group_options`) | Robust average excluding extremes |
| `trimmed-median` | Median after trimming outliers (set trim % in `time_group_options`) | Robust median excluding extremes |
| `extremes` | Min and max values | Show value range per interval |

:::important

When using `time_group` values other than `min`, `max`, `average`, or `sum`, you MUST specify `"tier": 0` in the `window` object to ensure a non-aggregated storage tier is used. Without it, the query may use a pre-aggregated tier (per-minute or per-hour) where advanced functions like `median`, `stddev`, `ses`, `des`, `percentile`, `countif`, `trimmed-mean`, `trimmed-median`, and `extremes` cannot work correctly.

:::

**Get context information**

```console
curl -H 'Accept: application/json' -H "Authorization: Bearer <token>" https://app.netdata.cloud/api/v2/contexts
```
