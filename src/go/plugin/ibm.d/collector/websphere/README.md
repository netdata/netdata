# IBM WebSphere Application Server collector

## Overview

This collector monitors IBM WebSphere Application Server performance metrics via REST API.

Currently supported:

- **WebSphere Liberty** - Full support via monitor-1.0 feature
- **WebSphere Traditional** - Version 8.5.5+ with REST API enabled

It collects:

**Global metrics:**

- JVM performance (heap, GC, threads, classes)
- Web container statistics (sessions, requests, errors)

**Per-instance metrics:**

- Thread pools (size, active, hung threads)
- Connection pools (size, wait time, timeouts)
- Web applications (requests, response time, errors)

## Requirements

For WebSphere Liberty:

- Liberty server with `monitor-1.0` feature enabled
- User with administrator or reader role

For WebSphere Traditional (8.5.5+):

- REST API must be enabled
- Monitoring role configured for the user

## Configuration

```yaml
jobs:
  - name: liberty_local
    url: https://localhost:9443
    username: admin
    password: password
```

## Metrics

All metrics have "websphere." prefix.

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| jvm_heap_usage | used, committed, max | MiB |
| jvm_gc_time | gc_time | milliseconds |
| jvm_gc_count | collections | collections/s |
| jvm_threads | threads, daemon, peak | threads |
| jvm_classes | loaded, unloaded | classes |
| web_sessions | active, live, invalidated | sessions |
| web_requests | requests | requests/s |
| web_errors | 4xx, 5xx | errors/s |

Per thread pool:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| threadpool_size | size, max | threads |
| threadpool_active | active | threads |
| threadpool_hung | hung | threads |

Per connection pool:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| connpool_size | size, free, max | connections |
| connpool_wait_time | avg_wait | milliseconds |
| connpool_timeouts | timeouts | timeouts/s |

Per application:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| app_requests | requests | requests/s |
| app_response_time | avg_response | milliseconds |
| app_errors | errors | errors/s |

## Setup

### Enable monitoring in Liberty

Add the monitor feature to your `server.xml`:

```xml
<server>
    <featureManager>
        <feature>monitor-1.0</feature>
    </featureManager>
</server>
```

### Create monitoring user

```xml
<server>
    <basicRegistry>
        <user name="monitor" password="{xor}..." />
    </basicRegistry>
    <administrator-role>
        <user>monitor</user>
    </administrator-role>
</server>
```

### Test connectivity

```bash
curl -k -u admin:password https://localhost:9443/ibm/api/metrics
```

## Troubleshooting

### Connection failures

1. Verify the WebSphere server is running and the REST API is accessible
2. Check firewall rules for the HTTPS port (default 9443)
3. Validate credentials have the proper role
4. For self-signed certificates, you may need to configure `tls_skip_verify: true`

### No metrics collected

1. Ensure the monitor-1.0 feature is enabled (Liberty)
2. Check the metrics endpoint is accessible: `curl -k -u user:pass https://server:9443/ibm/api/metrics`
3. Verify the user has administrator or reader role
