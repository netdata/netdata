# Netdata Alternative UI

A lightweight metrics aggregation server that can receive metrics from multiple nodes or applications and display them in a unified dashboard.

## Features

- **Push-based metrics collection**: Receive metrics via HTTP POST from any application or node
- **Kubernetes observability**: Dedicated Kubernetes collector and dashboard views
- **Real-time updates**: WebSocket-based live updates in the dashboard
- **Multi-node visualization**: View metrics from multiple sources in one place
- **Namespace filtering**: Filter Kubernetes metrics by namespace and resource type
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

## Kubernetes Monitoring

The Alternative UI includes a dedicated Kubernetes collector that gathers metrics from your clusters.

### Features

- **Node metrics**: Status, CPU usage, memory usage, capacity
- **Pod metrics**: Status by phase, CPU/memory per pod, container restarts
- **Deployment metrics**: Desired vs ready replicas
- **Cluster overview**: Namespace count, service count, PVC status

### Build the Kubernetes Collector

```bash
cd k8s
go mod tidy
go build -o k8s-collector .
```

### Run the Collector

```bash
# Using kubeconfig (for local development)
./k8s-collector \
  --url http://localhost:19998 \
  --kubeconfig ~/.kube/config \
  --cluster-name my-cluster \
  --interval 10s

# In-cluster (for running inside Kubernetes)
./k8s-collector \
  --url http://metrics-server:19998 \
  --cluster-name production \
  --namespaces default,kube-system,monitoring
```

### Collector Options

```
Options:
  -url string
        Alternative UI server URL (default "http://localhost:19998")
  -kubeconfig string
        Path to kubeconfig file (optional, uses in-cluster config if not set)
  -api-key string
        API key for authentication (optional)
  -cluster-name string
        Cluster name for identification (default "kubernetes")
  -interval duration
        Collection interval (default 10s)
  -namespaces string
        Comma-separated list of namespaces to monitor (empty = all)
```

### Kubernetes Dashboard Features

The web UI automatically detects Kubernetes clusters (nodes with `os: kubernetes` label) and provides:

- **Cluster sidebar section**: Separate listing for K8s clusters
- **Overview cards**: Quick view of nodes, pods, deployments, services count
- **Namespace filter**: Filter charts by namespace
- **Resource type filter**: Filter by cluster/nodes/pods/deployments/storage
- **Grouped charts**: Charts organized by resource family

### Metrics Collected

| Category | Chart | Description |
|----------|-------|-------------|
| Cluster | Namespaces | Total namespace count |
| Cluster | Services | Total service count |
| Cluster | ConfigMaps & Secrets | Config object counts |
| Cluster | PVC Status | Bound/Pending/Lost PVCs |
| Nodes | Status | Node ready status (1=ready, 0=not ready) |
| Nodes | CPU Usage | CPU usage per node (millicores) |
| Nodes | Memory Usage | Memory usage per node (MiB) |
| Nodes | Capacity | CPU and memory capacity |
| Pods | Status | Pod count by phase per namespace |
| Pods | CPU Usage | Top 20 pods by CPU |
| Pods | Memory Usage | Top 20 pods by memory |
| Pods | Restarts | Container restart counts |
| Deployments | Replicas | Desired replica count |
| Deployments | Ready | Ready replica count |

### Requirements

The Kubernetes collector requires:
- Access to Kubernetes API server
- `metrics-server` for CPU/memory metrics (optional but recommended)
- RBAC permissions to read pods, nodes, deployments, services, namespaces, configmaps, secrets, PVCs

### Example RBAC Configuration

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: k8s-collector
  namespace: monitoring
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: k8s-collector
rules:
- apiGroups: [""]
  resources: ["nodes", "pods", "services", "namespaces", "configmaps", "secrets", "persistentvolumeclaims"]
  verbs: ["list", "get"]
- apiGroups: ["apps"]
  resources: ["deployments", "replicasets"]
  verbs: ["list", "get"]
- apiGroups: ["metrics.k8s.io"]
  resources: ["nodes", "pods"]
  verbs: ["list", "get"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: k8s-collector
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: k8s-collector
subjects:
- kind: ServiceAccount
  name: k8s-collector
  namespace: monitoring
```

## k0s Kubernetes Distribution Support

The collector includes native support for [k0s](https://k0sproject.io/), a lightweight Kubernetes distribution. k0s clusters are automatically detected and additional metrics are collected.

### Auto-Detection

The collector automatically detects k0s clusters through:

1. **Node labels**: `node.k0sproject.io/role` (controller/worker)
2. **Node annotations**: `k0s.k0sproject.io/version`
3. **System pods**: Presence of konnectivity-agent or k0s-* pods

### k0s-Specific Kubeconfig

When no kubeconfig is specified, the collector automatically checks for the k0s default location:

```bash
/var/lib/k0s/pki/admin.conf
```

### Running on k0s

```bash
# On a k0s controller node (uses default kubeconfig)
./k8s-collector \
  --url http://localhost:19998 \
  --cluster-name my-k0s-cluster \
  --interval 10s

# With explicit k0s kubeconfig
./k8s-collector \
  --url http://localhost:19998 \
  --kubeconfig /var/lib/k0s/pki/admin.conf \
  --cluster-name production-k0s
```

### k0s Metrics Collected

In addition to standard Kubernetes metrics, the following k0s-specific metrics are collected:

| Category | Chart | Description |
|----------|-------|-------------|
| k0s | Cluster Topology | Controller and worker node counts |
| k0s | System Components | Status of k0s system components (1=healthy, 0=unhealthy) |
| k0s | System Replicas | Desired vs ready replicas for system components |
| k0s | Control Plane CPU | CPU usage of control plane components (millicores) |
| k0s | Control Plane Memory | Memory usage of control plane components (MiB) |
| k0s | Konnectivity Status | Status of konnectivity agents per node |
| k0s | Etcd Status | Embedded etcd pod status |
| k0s | Etcd CPU | Etcd CPU usage (if metrics available) |
| k0s | Etcd Memory | Etcd memory usage (if metrics available) |
| k0s | Node Roles | Nodes organized by k0s role |

### k0s System Components Monitored

The collector monitors the following k0s system components in the kube-system namespace:

- **coredns**: Cluster DNS
- **konnectivity-agent**: Node-to-control-plane tunnel
- **konnectivity-server**: Control plane tunnel server
- **metrics-server**: Resource metrics API
- **calico-node**: Calico CNI (if installed)
- **kube-router**: Default k0s CNI

### k0s Labels

When a k0s cluster is detected, the pushed metrics include additional labels:

```json
{
  "os": "k0s 1.28.2+k0s.0",
  "labels": {
    "type": "kubernetes",
    "distribution": "k0s",
    "k0s_version": "1.28.2+k0s.0",
    "cluster": "my-cluster"
  }
}
```

### Dashboard Display

The web UI shows k0s-specific information:

- Cluster displays as "k0s" or "k0s <version>" in the sidebar
- k0s metrics are grouped under the "k0s" family
- System component health is visualized with status indicators

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
