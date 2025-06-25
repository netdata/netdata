# IBM WebSphere Liberty MicroProfile Metrics collector

## Overview

This collector monitors IBM WebSphere Liberty servers with MicroProfile Metrics enabled, providing comprehensive performance monitoring through the standardized MicroProfile Metrics API.

Supported features:

- **MicroProfile Metrics 4.0+** - Full support for base, vendor, and application metrics
- **Prometheus format** - Native support for Prometheus-format metrics endpoint
- **Dynamic metric discovery** - Automatic detection and charting of new metrics
- **Cardinality control** - Configurable limits to prevent metric explosion

It collects:

**JVM metrics:**
- Memory usage (heap, non-heap, pools)
- Garbage collection statistics
- Thread counts and states
- Class loading metrics

**REST endpoint metrics:**
- Request counts and rates
- Response times and percentiles
- Error rates by endpoint and method

**MicroProfile component metrics:**
- Health check status
- Configuration values
- Fault tolerance statistics
- Custom application metrics

## Requirements

- WebSphere Liberty with MicroProfile Metrics feature enabled
- Liberty version 20.0.0.3+ (supports mpMetrics-3.0 or higher)
- Network access to Liberty metrics endpoint
- Appropriate user credentials (if authentication is enabled)

## Configuration

```yaml
jobs:
  - name: liberty_mp_local
    url: https://localhost:9443
    username: admin
    password: password
```

## Metrics

All metrics have "websphere_mp." prefix.

### JVM Metrics

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| jvm_memory_heap | used, committed | bytes |
| jvm_memory_heap_max | max | bytes |
| jvm_gc_collections | collections | collections/s |
| jvm_threads | threads, daemon, max | threads |
| jvm_classes | loaded, unloaded | classes |

### REST Endpoint Metrics (per endpoint)

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| rest_requests | requests | requests/s |
| rest_timing | response_time | milliseconds |

### MicroProfile Component Metrics

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| mp_health | status | status |
| mp_metrics | value | value |

### Custom Application Metrics (per application)

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| application | value | value |

## Setup

### Enable MicroProfile Metrics in Liberty

Add the MicroProfile Metrics feature to your `server.xml`:

```xml
<server>
    <featureManager>
        <feature>mpMetrics-4.0</feature>
        <!-- Other features -->
    </featureManager>
    
    <!-- Optional: Configure metrics endpoint -->
    <mpMetrics authentication="false" />
</server>
```

### Configure authentication (recommended for production)

```xml
<server>
    <basicRegistry>
        <user name="monitor" password="{xor}...encoded..." />
    </basicRegistry>
    
    <administrator-role>
        <user>monitor</user>
    </administrator-role>
    
    <!-- Secure metrics endpoint -->
    <mpMetrics authentication="true" />
</server>
```

### Test connectivity

```bash
# Without authentication
curl -k https://localhost:9443/metrics

# With authentication
curl -k -u monitor:password https://localhost:9443/metrics
```

## Configuration Examples

### Basic monitoring

```yaml
jobs:
  - name: liberty_mp_basic
    url: https://localhost:9443
    username: admin
    password: adminpwd
```

### Production setup with filtering

```yaml
jobs:
  - name: liberty_mp_prod
    url: https://prod.example.com:9443
    username: monitor
    password: secret
    
    # Only monitor API endpoints
    collect_rest_matching: "/api/*"
    max_rest_endpoints: 20
    
    # Enable custom application metrics
    collect_custom_metrics: true
    collect_custom_matching: "myapp_*"
    max_custom_metrics: 50
```

### Multiple servers

```yaml
jobs:
  - name: liberty_mp_server1
    url: https://server1.example.com:9443
    username: monitor
    password: secret
    
  - name: liberty_mp_server2
    url: https://server2.example.com:9443
    username: monitor
    password: secret
    update_every: 10
```

### TLS client certificate authentication

```yaml
jobs:
  - name: liberty_mp_secure
    url: https://secure.example.com:9443
    tls_cert: /path/to/client.crt
    tls_key: /path/to/client.key
    tls_ca: /path/to/ca.crt
```

### Minimal JVM-only monitoring

```yaml
jobs:
  - name: liberty_mp_minimal
    url: https://localhost:9443
    username: admin
    password: adminpwd
    
    # Disable dynamic metrics to reduce overhead
    collect_rest_metrics: false
    collect_mp_metrics: false
    collect_custom_metrics: false
```

## Troubleshooting

### Connection issues

1. **Connection refused**
   - Verify Liberty is running and port is correct
   - Check firewall rules
   - Ensure MicroProfile Metrics feature is enabled

2. **SSL/TLS errors**
   - Use `tls_skip_verify: true` for testing (not production)
   - Provide proper CA certificate with `tls_ca`
   - Verify certificate validity

### Authentication problems

1. **401 Unauthorized**
   - Verify username and password
   - Check user has reader or administrator role
   - Verify `mpMetrics` configuration in server.xml

2. **403 Forbidden**
   - User lacks sufficient permissions
   - Check role assignments in Liberty configuration

### Metrics issues

1. **404 Not Found for /metrics**
   - Verify `mpMetrics` feature is enabled
   - Check if custom metrics endpoint is configured
   - Ensure Liberty version supports MicroProfile Metrics

2. **No custom metrics**
   - Enable `collect_custom_metrics: true`
   - Check application is publishing metrics correctly
   - Verify metric names match filtering patterns

3. **Missing REST endpoints**
   - Verify JAX-RS applications are deployed
   - Check if endpoints have been accessed (some metrics only appear after first request)
   - Review `collect_rest_matching` pattern

### Performance considerations

1. **High memory usage**
   - Reduce `max_rest_endpoints` and `max_custom_metrics`
   - Use filtering to monitor only critical metrics
   - Increase `update_every` to reduce collection frequency

2. **Slow dashboard**
   - High cardinality from many REST endpoints
   - Consider filtering endpoints by importance
   - Monitor only business-critical applications

## Differences from regular WebSphere collector

| Feature | websphere | websphere_mp |
|:--------|:----------|:-------------|
| Data format | JSON (REST API) | Prometheus (MicroProfile) |
| Endpoint | `/ibm/api/metrics` | `/metrics` |
| Metric types | Fixed Liberty metrics | Extensible MP metrics |
| Custom metrics | Not supported | Full support |
| REST endpoint details | Basic counts | Detailed timing/percentiles |
| Standards compliance | Liberty-specific | MicroProfile standard |

## Advanced configuration

### Custom metric patterns

Use filtering to control which metrics are collected:

```yaml
# Only collect counters and gauges from myapp
collect_custom_matching: "application:myapp_counter_*|application:myapp_gauge_*"

# Only collect API endpoints, exclude health checks
collect_rest_matching: "/api/*"
```

### Cardinality management

For large applications with many endpoints:

```yaml
# Limit to most important endpoints
max_rest_endpoints: 10
collect_rest_matching: "/api/v1/users/*|/api/v1/orders/*"

# Limit custom metrics
max_custom_metrics: 25
collect_custom_matching: "*business_*|*critical_*"
```