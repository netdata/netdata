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

| Scope                  | Description                                                                                                                                        | API Access                         |
|:-----------------------|:---------------------------------------------------------------------------------------------------------------------------------------------------|:-----------------------------------|
| `scope:all`            | Grants the same permissions as the user who created the token. Use case: Terraform provider integration.                                           | Full access to all API endpoints   |
| `scope:agent-ui`       | Used by Agent for accessing the Cloud UI                                                                                                           | Access to UI-related endpoints     |
| `scope:grafana-plugin` | Used for the [Netdata Grafana plugin](https://github.com/netdata/netdata-grafana-datasource-plugin/blob/master/README.md) to access Netdata charts | Access to chart and data endpoints |
| `scope:mcp`            | Used to connect MCP clients (Claude Desktop, Cursor, etc.) to [Netdata Cloud MCP](/docs/netdata-ai/mcp/README.md#netdata-cloud-mcp) for AI-assisted monitoring | Access to MCP server endpoints     |

## API Versions

Netdata provides three API versions that you can access with API tokens:

- **v1**: The original API, focused on single-node operations
- **v2**: Multi-node API with advanced grouping and aggregation capabilities
- **v3**: The latest API version that combines v1 and v2 endpoints and may include additional features

## Common Endpoints

### Netdata Cloud endpoints

These endpoints are available when querying via `app.netdata.cloud`:

| Endpoint | Description | Method |
|:---------|:------------|:-------|
| `/api/v2/spaces` | Space listing | GET |
| `/api/v2/nodes` | Node information | GET |
| `/api/v2/data` | Multi-dimensional data queries | GET |
| `/api/v2/contexts` | Context metadata | GET |
| `/api/v2/weights` | Metric scoring/correlation | GET |
| `/api/v2/q` | Full-text search | GET |
| `/api/v3/spaces/{spaceID}/rooms/{roomID}/data` | Advanced metric queries with aggregation | POST |

### Agent-only endpoints

These endpoints are available only on local Netdata Agent instances and cannot be accessed through Netdata Cloud:

| Endpoint | Description |
|:---------|:------------|
| `/api/v1/info` | Agent information |
| `/api/v1/charts` | Chart information |
| `/api/v1/data` | Single node data queries |

:::note

v1 and v2 APIs are deprecated and maintained for backwards compatibility only. New integrations should use v3 exclusively. For Cloud metric queries, the POST endpoint `/api/v3/spaces/{spaceID}/rooms/{roomID}/data` is recommended.

:::

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

## Troubleshooting

### HTTP 404 errors

If you receive a 404 when querying Netdata Cloud, you may be using an Agent-only endpoint. v1 endpoints (`/api/v1/info`, `/api/v1/charts`, `/api/v1/data`) are only available on local Agent instances, not through Cloud. Use the Cloud-compatible endpoints listed in [Common Endpoints](#common-endpoints) instead.

### HTTP 401/403 errors

Verify your API token is valid and has the correct scope. Check the [Available Scopes](#available-scopes) table to ensure your token grants access to the required endpoints.

### Empty results

For data queries returning empty results, verify that `contexts` is set in your query parameters (for GET requests) or that `scope.contexts` is set in your payload (for POST requests). Also ensure the target nodes are online and streaming data.
