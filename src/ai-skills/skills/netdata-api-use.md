# Netdata API Usage

## Purpose

Query Netdata's REST API to retrieve metrics, contexts, alerts, and system information.

## Base URL

```
http://localhost:19999/api/v3/
```

For remote nodes or parents, replace `localhost` with the appropriate hostname/IP.

## Contexts API

Get available contexts (metric types) and their instances (charts).

### Endpoints

```bash
# Contexts only
curl -s "http://localhost:19999/api/v3/contexts"

# Contexts + instances (chart IDs) + dimensions
curl -s "http://localhost:19999/api/v3/contexts?options=instances,dimensions"

# With human-readable timestamps
curl -s "http://localhost:19999/api/v3/contexts?options=instances,retention,rfc3339"

# Filter by node(s)
curl -s "http://localhost:19999/api/v3/contexts?scope_nodes=hostname1,hostname2&options=instances"

# Filter by context pattern
curl -s "http://localhost:19999/api/v3/contexts?scope_contexts=disk.*&options=instances"

# Combined filters
curl -s "http://localhost:19999/api/v3/contexts?scope_nodes=myserver&scope_contexts=disk.space&options=instances,dimensions,labels"
```

### Options

| Option | Effect |
|--------|--------|
| `instances` | Include chart IDs (instances) |
| `dimensions` | Include dimension names |
| `labels` | Include chart labels |
| `retention` | Include first/last entry timestamps |
| `liveness` | Include live status |
| `family` | Include family |
| `units` | Include units |
| `priorities` | Include priorities |
| `titles` | Include titles |
| `rfc3339` | Human-readable timestamps |
| `long-keys` | Full JSON key names |
| `minify` | Compact JSON output |

### Filter Parameters

| Parameter | Effect |
|-----------|--------|
| `scope_nodes` | Filter by node hostname(s), comma-separated |
| `scope_contexts` | Filter by context pattern (supports wildcards) |
| `after` / `before` | Time range (unix timestamp) |
| `timeout` | Timeout in ms |
| `cardinality` | Limit number of results |

### Response Structure

```json
{
  "api": 2,
  "nodes": [
    {
      "mg": "machine-guid",
      "nd": "node-id",
      "nm": "hostname",
      "ni": 0
    }
  ],
  "contexts": {
    "disk.space": {
      "family": "/[x]",
      "units": "GiB",
      "priority": 2023,
      "first_entry": 1766901600,
      "last_entry": 1769292789,
      "live": true,
      "instances": [
        "disk_space./",
        "disk_space./tmp",
        "disk_space./home"
      ]
    }
  }
}
```

## Alerts API

Query current alert states and configurations.

### Endpoints

```bash
# All alerts (nodes list only)
curl -s "http://localhost:19999/api/v3/alerts"

# All alerts with instances and values
curl -s "http://localhost:19999/api/v3/alerts?options=instances,values"

# Filter by alert name
curl -s "http://localhost:19999/api/v3/alerts?alert=disk_space_usage&options=instances,values"

# Filter by status
curl -s "http://localhost:19999/api/v3/alerts?status=warning&options=instances"
curl -s "http://localhost:19999/api/v3/alerts?status=critical&options=instances"

# Filter by node
curl -s "http://localhost:19999/api/v3/alerts?scope_nodes=myserver&options=instances,values"

# Combined filters
curl -s "http://localhost:19999/api/v3/alerts?scope_nodes=myserver&alert=disk_space_usage&options=instances,values,config"
```

### Alert-Specific Parameters

| Parameter | Effect |
|-----------|--------|
| `alert` | Filter by alert name |
| `status` | Filter by status: `warning`, `critical`, `clear` |
| `transition` | Filter by transition ID |

### Alert Options

| Option | Effect |
|--------|--------|
| `instances` | Include alert instances in response |
| `values` | Include current values |
| `config` | Include configuration source |
| `summary` | Include summary counts |

### Response Structure

```json
{
  "api": 2,
  "nodes": [...],
  "alert_instances": [
    {
      "ni": 0,
      "gi": 1768964039135045,
      "nm": "disk_space_usage",
      "ctx": "disk.space",
      "ch": "disk_space./",
      "ch_n": "disk_space./",
      "st": "CLEAR",
      "fami": "/",
      "info": "Total space utilization of disk ${label:mount_point}",
      "sum": "Disk / space usage",
      "units": "%",
      "src": "line=11,file=/etc/netdata/health.d/disks.conf",
      "to": "sysadmin",
      "tp": "System",
      "cm": "Disk",
      "cl": "Utilization",
      "v": 79.56,
      "t": 1769292816
    }
  ]
}
```

### Response Fields

| Field | Meaning |
|-------|---------|
| `ni` | Node index |
| `gi` | Global ID |
| `nm` | Alert name |
| `ctx` | Context |
| `ch` | Chart ID |
| `ch_n` | Chart name |
| `st` | Status: `CLEAR`, `WARNING`, `CRITICAL` |
| `fami` | Family |
| `info` | Alert info text |
| `sum` | Summary text |
| `units` | Units |
| `src` | Source file and line |
| `to` | Notification recipient |
| `tp` | Type |
| `cm` | Component |
| `cl` | Class |
| `v` | Current value |
| `t` | Timestamp |

## Common Use Cases

### Find Chart ID for a Specific Mount Point

```bash
curl -s "http://localhost:19999/api/v3/contexts?scope_contexts=disk.space&options=instances" | jq '.contexts["disk.space"].instances'
```

### Check if Alert Override is Active

```bash
curl -s "http://localhost:19999/api/v3/alerts?alert=disk_space_usage&options=instances,config" | jq '.alert_instances[0].src'
```

The `src` field shows the config file path. User overrides are in `/etc/netdata/health.d/`, stock configs in `/usr/lib/netdata/conf.d/health.d/`.

### Get All Critical Alerts

```bash
curl -s "http://localhost:19999/api/v3/alerts?status=critical&options=instances,values" | jq '.alert_instances[] | {name: .nm, chart: .ch, value: .v, info: .sum}'
```

### Get Dimensions for a Context

```bash
curl -s "http://localhost:19999/api/v3/contexts?scope_contexts=system.cpu&options=dimensions" | jq '.contexts["system.cpu"]'
```

### List All Contexts on a Node

```bash
curl -s "http://localhost:19999/api/v3/contexts?scope_nodes=myserver" | jq '.contexts | keys'
```

### Get Alert History/Transitions

```bash
curl -s "http://localhost:19999/api/v3/alert_transitions?alert=disk_space_usage&last=10"
```

## Data Query API

Query time-series data from charts.

### Endpoints

```bash
# Query data for a chart
curl -s "http://localhost:19999/api/v3/data?contexts=system.cpu&after=-300"

# Multiple contexts
curl -s "http://localhost:19999/api/v3/data?contexts=system.cpu,system.load&after=-300"

# Specific dimensions
curl -s "http://localhost:19999/api/v3/data?contexts=system.cpu&dimensions=user,system&after=-300"

# With aggregation
curl -s "http://localhost:19999/api/v3/data?contexts=system.cpu&after=-3600&points=60&group=average"
```

### Query Parameters

| Parameter | Effect |
|-----------|--------|
| `contexts` | Context(s) to query, comma-separated |
| `scope_nodes` | Filter by node(s) |
| `dimensions` | Specific dimensions to include |
| `after` | Start time (negative = seconds ago, or unix timestamp) |
| `before` | End time (default: now) |
| `points` | Number of points to return |
| `group` | Aggregation: `average`, `min`, `max`, `sum` |
| `format` | Output format: `json`, `csv`, `tsv` |

## Error Handling

### HTTP Status Codes

| Code | Meaning |
|------|---------|
| 200 | Success |
| 400 | Bad request (invalid parameters) |
| 404 | Context/chart not found |
| 503 | Service unavailable |

### Check Node Status

The response includes node status:

```json
"st": {
  "ai": 0,
  "code": 200,
  "msg": ""
}
```

- `code: 200` = OK
- `code: 503` = Node unavailable
