# Kubernetes API Server collector

## Overview

This collector monitors Kubernetes API Server health, performance, and request metrics by scraping
the Prometheus-format metrics from the kube-apiserver's `/metrics` endpoint.

## Collected metrics

### Requests
- `k8s_apiserver.requests_total` - Total API request rate
- `k8s_apiserver.requests_dropped` - Dropped requests (due to overload)
- `k8s_apiserver.requests_by_verb` - Requests by HTTP verb (GET, POST, PUT, DELETE, PATCH, LIST, WATCH)
- `k8s_apiserver.requests_by_code` - Requests by HTTP status code
- `k8s_apiserver.requests_by_resource` - Requests by Kubernetes resource type

### Latency
- `k8s_apiserver.request_latency` - Request latency percentiles (p50, p90, p99)
- `k8s_apiserver.response_size` - Response size percentiles

### Inflight
- `k8s_apiserver.inflight_requests` - Current inflight requests (mutating vs read-only)
- `k8s_apiserver.longrunning_requests` - Long-running requests (WATCH, etc.)

### REST Client
- `k8s_apiserver.rest_client_requests_by_code` - REST client requests by status code
- `k8s_apiserver.rest_client_requests_by_method` - REST client requests by HTTP method
- `k8s_apiserver.rest_client_latency` - REST client latency percentiles

### Admission
- `k8s_apiserver.admission_step_latency` - Admission step latency (validate/admit)
- `k8s_apiserver.admission_controller_latency` - Per-controller latency (dynamic)
- `k8s_apiserver.admission_webhook_latency` - Per-webhook latency (dynamic)

### Etcd
- `k8s_apiserver.etcd_object_counts` - Objects stored in etcd by resource type

### Workqueue
- `k8s_apiserver.workqueue_depth` - Controller work queue depth (dynamic per controller)
- `k8s_apiserver.workqueue_latency` - Queue latency percentiles (dynamic per controller)
- `k8s_apiserver.workqueue_adds` - Work queue adds and retries (dynamic per controller)
- `k8s_apiserver.workqueue_duration` - Work duration percentiles (dynamic per controller)

### Audit
- `k8s_apiserver.audit_events` - Audit events generated

### Authentication
- `k8s_apiserver.authentication_requests` - Authenticated requests

### Process
- `k8s_apiserver.goroutines` - Go runtime goroutines
- `k8s_apiserver.threads` - OS threads
- `k8s_apiserver.process_memory` - Process memory (resident/virtual)
- `k8s_apiserver.heap_memory` - Go heap memory
- `k8s_apiserver.gc_duration` - GC duration percentiles
- `k8s_apiserver.open_fds` - Open file descriptors
- `k8s_apiserver.cpu_usage` - CPU usage

## Configuration

Edit the `go.d/k8s_apiserver.conf` configuration file using `edit-config` from the
Netdata [config directory](/docs/netdata-agent/configuration/README.md), which is typically
at `/etc/netdata`.

```bash
cd /etc/netdata # Replace with your Netdata config directory if different
sudo ./edit-config go.d/k8s_apiserver.conf
```

### In-cluster configuration

When running inside a Kubernetes cluster, the default configuration should work:

```yaml
jobs:
  - name: local
    url: https://kubernetes.default.svc:443/metrics
    bearer_token_file: /var/run/secrets/kubernetes.io/serviceaccount/token
    tls_ca: /var/run/secrets/kubernetes.io/serviceaccount/ca.crt
```

### External access via kubectl proxy

```yaml
jobs:
  - name: via-proxy
    url: http://127.0.0.1:8001/metrics
```

### Direct external access

```yaml
jobs:
  - name: direct
    url: https://api.example.com:6443/metrics
    bearer_token_file: /path/to/token
    tls_skip_verify: yes  # or provide tls_ca
```

## Requirements

The ServiceAccount used must have permissions to access the `/metrics` endpoint.
You may need to create a ClusterRole and ClusterRoleBinding:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: netdata-apiserver-metrics
rules:
  - nonResourceURLs:
      - /metrics
    verbs:
      - get

---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: netdata-apiserver-metrics
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: netdata-apiserver-metrics
subjects:
  - kind: ServiceAccount
    name: netdata
    namespace: netdata
```

## Troubleshooting

### Connection refused
- Verify the URL is correct
- Check network policies allow access
- Ensure the ServiceAccount has proper RBAC permissions

### 401 Unauthorized
- Verify the bearer token file exists and is readable
- Check the token is valid and not expired
- Ensure the ServiceAccount has metrics access permissions

### Certificate errors
- Provide the correct CA certificate path in `tls_ca`
- Or set `tls_skip_verify: yes` (not recommended for production)

## Cardinality Limits

To prevent excessive memory usage in large clusters, the collector enforces cardinality limits
on dynamic dimensions:

| Dimension Type       | Maximum | Notes                                      |
|---------------------|---------|---------------------------------------------|
| Resources           | 500     | Kubernetes resource types (pods, services, etc.) |
| Work Queues         | 100     | Controller work queues                      |
| Admission Controllers | 100   | Admission controller names                  |
| Admission Webhooks  | 50      | Admission webhook names                     |

When these limits are reached, new dimensions are silently ignored. Existing dimensions that
haven't been seen for approximately 5 minutes (300 collection cycles) are automatically cleaned
up to make room for new ones.

If you need to monitor more dimensions, you can modify the `defaultMax*` constants in the
collector source code.
