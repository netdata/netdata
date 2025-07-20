# IBM WebSphere Liberty MicroProfile Metrics collector

## Overview

This collector monitors IBM WebSphere Liberty servers with MicroProfile Metrics enabled, providing performance monitoring through the standardized MicroProfile Metrics API in Prometheus format.

It collects:

**JVM metrics:**
- Memory usage (heap used, free, committed, max)
- Heap utilization percentage
- Garbage collection (rate and time)
- Thread counts (current daemon/other, peak)
- Class loading (loaded classes, unload rate)
- CPU usage (process load, utilization percentage)
- CPU time and available processors
- System load average

**Vendor-specific metrics:**
- Thread pool usage (active, idle, size)
- Servlet performance (request rate, response time)
- Session management (active, live, lifecycle)

**Other metrics:**
- Any additional metrics exposed by the MicroProfile Metrics endpoint
- Collected as generic "other" category metrics

## Requirements

- WebSphere Liberty with MicroProfile Metrics feature enabled
- Liberty version 20.0.0.3+ (supports mpMetrics-3.0 or higher)
- Network access to Liberty metrics endpoint (default: /metrics)
- Appropriate user credentials (if authentication is enabled)

## Configuration

```yaml
jobs:
  - name: liberty_mp_local
    url: https://localhost:9443
    username: admin
    password: password
```

### URL Configuration

The collector supports flexible URL configuration:

**Option 1: Base URL + metrics endpoint (recommended)**
```yaml
url: https://localhost:9443
metrics_endpoint: /metrics    # Default, auto-appended
```

**Option 2: Full URL with path**
```yaml
url: https://localhost:9443/metrics
# metrics_endpoint automatically detected and used as-is
```

**Option 3: Custom metrics path**
```yaml
url: https://localhost:9443
metrics_endpoint: /custom/metrics/path
```

### All available options

```yaml
  - name: liberty_mp_example
    url: https://localhost:9443          # Required: base URL or full URL
    username: admin                       # Optional
    password: password                    # Optional
    metrics_endpoint: /metrics            # Default: /metrics (auto-appended if needed)
    collect_jvm_metrics: true            # Default: true
    collect_rest_metrics: true           # Default: true
    max_rest_endpoints: 50               # Default: 50 (0 = unlimited)
    collect_rest_matching: '/api/*'      # Optional: filter REST metrics
    timeout: 10                          # Default: 10 seconds
    tls_skip_verify: false               # Default: false
    tls_ca: /path/to/ca.crt             # Optional
    tls_cert: /path/to/client.crt       # Optional
    tls_key: /path/to/client.key        # Optional
    cell_name: MyCell                    # Optional: for clustering
    node_name: MyNode                    # Optional: for clustering
    server_name: server1                 # Optional: for clustering
```

## Metrics

All metrics have the prefix `websphere_mp.`.

### JVM Metrics

| Metric | Description | Dimensions | Unit |
|--------|-------------|------------|------|
| jvm_memory_heap_usage | JVM heap memory usage | used, free | bytes |
| jvm_memory_heap_committed | JVM heap memory committed | committed | bytes |
| jvm_memory_heap_max | JVM heap memory maximum | limit | bytes |
| jvm_heap_utilization | JVM heap utilization | utilization | percentage |
| jvm_gc_collections | JVM garbage collection rate | rate | collections/s |
| jvm_gc_time | JVM garbage collection time | total, per_cycle | milliseconds |
| jvm_threads_current | Current JVM threads | daemon, other | threads |
| jvm_threads_peak | Peak JVM threads | peak | threads |
| jvm_classes_loaded | Loaded classes | loaded | classes |
| jvm_classes_unloaded_rate | Class unload rate | unloaded | classes/s |
| cpu_usage | JVM CPU usage | process, utilization | percentage |
| cpu_time | JVM CPU time | total | seconds |
| cpu_processors | Available processors | available | processors |
| system_load | System load average | 1min | load |

### Vendor Metrics

| Metric | Description | Dimensions | Unit |
|--------|-------------|------------|------|
| threadpool_usage | Thread pool usage | active, idle | threads |
| threadpool_size | Thread pool size | size | threads |
| servlet_requests | Servlet request rate | requests | requests/s |
| servlet_response_time | Servlet response time | avg_response_time | milliseconds |
| session_active | Active sessions | active, live | sessions |
| session_lifecycle | Session lifecycle | created, invalidated, timed_out | sessions/s |

### Other Metrics

Any metrics not matching JVM or vendor patterns are collected in the "other" family with appropriate units based on metric name suffixes (_bytes, _seconds, _percent, etc.).

## Troubleshooting

### Connection refused
- Verify the URL and port are correct
- Check if Liberty is running
- Ensure MicroProfile Metrics feature is enabled in server.xml:
  ```xml
  <featureManager>
      <feature>mpMetrics-3.0</feature>
  </featureManager>
  ```

### 401 Unauthorized
- Verify username and password
- Ensure user has reader or administrator role
- Check if metrics endpoint requires authentication

### 404 Not Found
- Verify metrics_endpoint path (default: /metrics)
- Ensure mpMetrics feature is enabled
- Check Liberty version supports MicroProfile Metrics

### High memory usage
- Reduce max_rest_endpoints
- Use collect_rest_matching to filter endpoints
- Increase update_every to reduce collection frequency

### Certificate errors
- For self-signed certificates, use: `tls_skip_verify: true` (not for production!)
- Provide proper CA certificate with tls_ca option
- Ensure certificate paths are absolute and readable

## Testing

To test MicroProfile Metrics connectivity:
```bash
curl -k -u admin:adminpwd https://localhost:9443/metrics
```

To test the collector:
```bash
cd /usr/libexec/netdata/plugins.d/
sudo -u netdata ./ibm.d.plugin -d -m websphere_mp
```

## Comparison with websphere collector

Use `websphere_mp` when:
- You have Liberty with MicroProfile Metrics enabled
- You want metrics in Prometheus format
- You need CPU metrics and thread pool monitoring
- You prefer the MicroProfile standard metric names

Use `websphere` when:
- You have Traditional WebSphere (not Liberty)
- You don't have MicroProfile Metrics enabled
- You need PMI-based metrics
- You want lower overhead collection