# Netdata Alternative UI

A lightweight metrics aggregation server that can receive metrics from multiple nodes or applications and display them in a unified dashboard.

## Features

- **Push-based metrics collection**: Receive metrics via HTTP POST from any application or node
- **Real-time updates**: WebSocket-based live updates in the dashboard
- **Multi-node visualization**: View metrics from multiple sources in one place
- **Configurable retention**: Set how long to keep historical data
- **Simple REST API**: Easy integration with any language or framework
- **Zero dependencies**: Single binary with embedded web UI

## Quick Start

### Build

```bash
cd src/alternative-ui
go build -o netdata-alt-ui .
```

### Run

```bash
./netdata-alt-ui -addr :19998
```

The dashboard will be available at http://localhost:19998

### Configuration Options

```
Usage:
  netdata-alt-ui [options]

Options:
  -addr string
        Listen address (default ":19998")
  -retention duration
        Data retention duration (default 1h)
  -max-points int
        Maximum data points per dimension (default 3600)
  -api-key string
        API key for push authentication (optional)
  -version
        Show version
```

## Pushing Metrics

### API Endpoint

```
POST /api/v1/push
Content-Type: application/json
X-API-Key: <your-api-key>  (if configured)
```

### Payload Format

```json
{
  "node_id": "my-app",
  "node_name": "My Application",
  "hostname": "server1.example.com",
  "os": "Linux 5.4.0",
  "labels": {
    "environment": "production",
    "region": "us-west-2"
  },
  "timestamp": 1700000000000,
  "charts": [
    {
      "id": "app.requests",
      "type": "app",
      "title": "HTTP Requests",
      "units": "requests/s",
      "family": "http",
      "chart_type": "area",
      "priority": 100,
      "dimensions": [
        {
          "id": "total",
          "name": "Total Requests",
          "value": 1234,
          "algorithm": "incremental"
        },
        {
          "id": "errors",
          "name": "Errors",
          "value": 5,
          "algorithm": "incremental"
        }
      ]
    }
  ]
}
```

### Field Descriptions

#### MetricPush (Root Object)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `node_id` | string | Yes | Unique identifier for the node/application |
| `node_name` | string | No | Display name for the node |
| `hostname` | string | No | Hostname of the machine |
| `os` | string | No | Operating system information |
| `labels` | object | No | Key-value labels for the node |
| `timestamp` | int64 | No | Unix timestamp in milliseconds |
| `charts` | array | Yes | Array of charts with metrics |

#### Chart Object

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique chart identifier |
| `type` | string | Chart type/category |
| `name` | string | Short name |
| `title` | string | Display title |
| `units` | string | Measurement units (e.g., %, bytes, requests/s) |
| `family` | string | Chart family/group |
| `context` | string | Chart context for grouping |
| `chart_type` | string | Visualization type: `line`, `area`, `stacked` |
| `priority` | int | Sort priority (lower = higher in list) |
| `update_every` | int | Update interval in seconds |
| `dimensions` | array | Array of metric dimensions |

#### Dimension Object

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique dimension identifier |
| `name` | string | Display name |
| `value` | float64 | Current metric value |
| `algorithm` | string | Value algorithm: `absolute` (default), `incremental` |
| `multiplier` | float64 | Value multiplier (default: 1) |
| `divisor` | float64 | Value divisor (default: 1) |

## REST API

### Health Check

```
GET /api/v1/health
```

Response:
```json
{
  "status": "healthy",
  "timestamp": 1700000000000,
  "nodes": 5
}
```

### List Nodes

```
GET /api/v1/nodes
```

Response:
```json
[
  {
    "id": "my-app",
    "name": "My Application",
    "hostname": "server1",
    "os": "Linux 5.4.0",
    "chart_count": 5,
    "last_seen": "2024-01-01T00:00:00Z",
    "online": true
  }
]
```

### Get Node Details

```
GET /api/v1/node/{node_id}
```

### List Charts for a Node

```
GET /api/v1/charts/{node_id}
```

### Get Chart Data

```
GET /api/v1/data/{node_id}/{chart_id}?after={timestamp}&before={timestamp}&points={count}
```

Query parameters:
- `after`: Start timestamp (Unix milliseconds)
- `before`: End timestamp (Unix milliseconds)
- `points`: Number of data points to return

## WebSocket API

Connect to `/ws` for real-time updates.

### Message Types

#### Received from Server

```json
{"type": "init", "payload": [...nodes...]}
{"type": "node_added", "payload": {...node...}}
{"type": "node_online", "payload": {"node_id": "..."}}
{"type": "node_offline", "payload": {"node_id": "..."}}
{"type": "metrics_update", "payload": {...metrics...}}
```

#### Send to Server

```json
{"type": "subscribe", "node_ids": ["node1", "node2"]}
{"type": "unsubscribe", "node_ids": ["node1"]}
{"type": "ping"}
```

## Examples

### cURL

```bash
curl -X POST http://localhost:19998/api/v1/push \
  -H "Content-Type: application/json" \
  -d '{
    "node_id": "test-node",
    "charts": [{
      "id": "cpu",
      "title": "CPU Usage",
      "units": "%",
      "dimensions": [
        {"id": "user", "value": 45.2},
        {"id": "system", "value": 12.8}
      ]
    }]
  }'
```

### Python

See `examples/push_metrics.py` for a complete system metrics collector.

```python
import requests

requests.post("http://localhost:19998/api/v1/push", json={
    "node_id": "my-python-app",
    "charts": [{
        "id": "app.requests",
        "title": "Requests",
        "units": "req/s",
        "dimensions": [
            {"id": "total", "value": request_count, "algorithm": "incremental"}
        ]
    }]
})
```

### Shell

See `examples/push_metrics.sh` for a dependency-free shell script collector.

### Go

See `examples/app_metrics.go` for a complete Go application example.

## Integration with Netdata

The alternative UI can receive metrics from Netdata agents using a simple exporter:

```bash
# Forward Netdata metrics to Alternative UI
curl -s "http://localhost:19999/api/v1/allmetrics?format=json" | \
  jq '{
    node_id: .hostname,
    charts: [.data[] | {
      id: .id,
      title: .title,
      units: .units,
      dimensions: [.dimensions[] | {id: .id, name: .name, value: .value}]
    }]
  }' | \
  curl -X POST -H "Content-Type: application/json" -d @- http://localhost:19998/api/v1/push
```

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Alternative UI Server                     │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────┐  │
│  │ HTTP Server  │  │  WebSocket   │  │  Metrics Store   │  │
│  │  - Push API  │  │    Hub       │  │  - In-memory     │  │
│  │  - Query API │  │  - Real-time │  │  - Configurable  │  │
│  │  - Static    │  │    updates   │  │    retention     │  │
│  └──────┬───────┘  └──────┬───────┘  └────────┬─────────┘  │
│         │                  │                   │            │
│         └──────────────────┴───────────────────┘            │
│                            │                                │
│  ┌─────────────────────────▼────────────────────────────┐  │
│  │                    Web Dashboard                      │  │
│  │  - Multi-node view    - Real-time charts             │  │
│  │  - Dark/Light theme   - Responsive design            │  │
│  └──────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────┘

        ▲                    ▲                    ▲
        │                    │                    │
┌───────┴──────┐    ┌───────┴───────┐    ┌──────┴───────┐
│   Node 1     │    │   Node 2      │    │  Application │
│  (Python)    │    │  (Go)         │    │  (Any lang)  │
└──────────────┘    └───────────────┘    └──────────────┘
```

## License

GPL-3.0-or-later - Same as Netdata
