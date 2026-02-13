# IBM WebSphere JMX collector

## Overview

Collects JVM, thread pool, and middleware metrics from IBM WebSphere Application Server
via the embedded JMX bridge helper.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM WebSphere JMX instance


These metrics refer to the entire monitored IBM WebSphere JMX instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.jvm_heap_memory | used, committed, max | bytes |
| websphere_jmx.jvm_heap_usage | usage | percentage |
| websphere_jmx.jvm_nonheap_memory | used, committed | bytes |
| websphere_jmx.jvm_gc_count | collections | collections |
| websphere_jmx.jvm_gc_time | time | milliseconds |
| websphere_jmx.jvm_threads | total, daemon | threads |
| websphere_jmx.jvm_thread_states | peak, started | threads |
| websphere_jmx.jvm_classes | loaded, unloaded | classes |
| websphere_jmx.jvm_process_cpu_usage | cpu | percentage |
| websphere_jmx.jvm_uptime | uptime | seconds |



### Per applications

These metrics refer to individual applications instances.

Labels:

| Label | Description |
|:------|:------------|
| application | Application identifier |
| module | Module identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.app_requests | requests | requests |
| websphere_jmx.app_response_time | response_time | milliseconds |
| websphere_jmx.app_sessions_active | active | sessions |
| websphere_jmx.app_sessions_live | live | sessions |
| websphere_jmx.app_session_events | creates, invalidates | sessions |
| websphere_jmx.app_transactions | committed, rolledback | transactions |

### Per jca

These metrics refer to individual jca instances.

Labels:

| Label | Description |
|:------|:------------|
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.jca_pool_size | size | connections |
| websphere_jmx.jca_pool_usage | active, free | connections |
| websphere_jmx.jca_pool_wait_time | wait | milliseconds |
| websphere_jmx.jca_pool_use_time | use | milliseconds |
| websphere_jmx.jca_pool_connections | created, destroyed | connections |
| websphere_jmx.jca_pool_waiting_threads | waiting | threads |

### Per jdbc

These metrics refer to individual jdbc instances.

Labels:

| Label | Description |
|:------|:------------|
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.jdbc_pool_size | size | connections |
| websphere_jmx.jdbc_pool_usage | active, free | connections |
| websphere_jmx.jdbc_pool_wait_time | wait | milliseconds |
| websphere_jmx.jdbc_pool_use_time | use | milliseconds |
| websphere_jmx.jdbc_pool_connections | created, destroyed | connections |
| websphere_jmx.jdbc_pool_waiting_threads | waiting | threads |

### Per jms

These metrics refer to individual jms instances.

Labels:

| Label | Description |
|:------|:------------|
| destination | Destination identifier |
| destination_type | Destination_type identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.jms_messages_current | current | messages |
| websphere_jmx.jms_messages_pending | pending | messages |
| websphere_jmx.jms_messages_total | total | messages |
| websphere_jmx.jms_consumers | consumers | consumers |

### Per threadpools

These metrics refer to individual threadpools instances.

Labels:

| Label | Description |
|:------|:------------|
| pool | Pool identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_jmx.threadpool_size | size, max | threads |
| websphere_jmx.threadpool_active | active | threads |


## Configuration

### File

The configuration file name for this integration is `ibm.d/websphere_jmx.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/websphere_jmx.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| update_every | Data collection frequency | `5` | no | 1 | - |
| Vnode | Vnode | `` | no | - | - |
| JMXURL | Connection settings | `` | no | - | - |
| JMXUsername | Username | `` | no | - | - |
| JMXPassword | Password | `` | no | - | - |
| JMXClasspath | J m x classpath | `` | no | - | - |
| JavaExecPath | Java exec path | `` | no | - | - |
| JMXTimeout | Timeout | `5000000000` | no | - | - |
| InitTimeout | Timeout | `30000000000` | no | - | - |
| ShutdownDelay | Shutdown delay | `100000000` | no | - | - |
| ClusterName | Identity labels | `` | no | - | - |
| CellName | Cell name | `` | no | - | - |
| NodeName | Node name | `` | no | - | - |
| ServerName | Server name | `` | no | - | - |
| ServerType | Server type | `` | no | - | - |
| CollectJVMMetrics | Metric toggles | `enabled` | no | - | - |
| CollectThreadPoolMetrics | Collect Thread pool metrics | `enabled` | no | - | - |
| CollectJDBCMetrics | Collect J d b c metrics | `enabled` | no | - | - |
| CollectJCAMetrics | Collect J c a metrics | `enabled` | no | - | - |
| CollectJMSMetrics | Collect J m s metrics | `enabled` | no | - | - |
| CollectWebAppMetrics | Collect Web app metrics | `enabled` | no | - | - |
| CollectSessionMetrics | Collect Session metrics | `enabled` | no | - | - |
| CollectTransactionMetrics | Collect Transaction metrics | `enabled` | no | - | - |
| CollectClusterMetrics | Collect Cluster metrics | `enabled` | no | - | - |
| CollectServletMetrics | Collect Servlet metrics | `enabled` | no | - | - |
| CollectEJBMetrics | Collect E j b metrics | `enabled` | no | - | - |
| CollectJDBCAdvanced | Collect J d b c advanced | `disabled` | no | - | - |
| MaxThreadPools | Cardinality guards | `50` | no | - | - |
| MaxJDBCPools | Max J d b c pools | `50` | no | - | - |
| MaxJCAPools | Max J c a pools | `50` | no | - | - |
| MaxJMSDestinations | Max J m s destinations | `50` | no | - | - |
| MaxApplications | Max Applications | `100` | no | - | - |
| MaxServlets | Max Servlets | `50` | no | - | - |
| MaxEJBs | Max E j bs | `50` | no | - | - |
| CollectPoolsMatching | Filters | `` | no | - | - |
| CollectJMSMatching | Collect J m s matching | `` | no | - | - |
| CollectAppsMatching | Collect Apps matching | `` | no | - | - |
| CollectServletsMatching | Collect Servlets matching | `` | no | - | - |
| CollectEJBsMatching | Collect E j bs matching | `` | no | - | - |
| MaxRetries | Resilience tuning | `3` | no | - | - |
| RetryBackoffMultiplier | Retry backoff multiplier | `2` | no | - | - |
| CircuitBreakerThreshold | Circuit breaker threshold | `5` | no | - | - |
| HelperRestartMax | Helper restart max | `3` | no | - | - |

### Examples

#### Basic configuration

IBM WebSphere JMX monitoring with default settings.

<details>
<summary>Config</summary>

```yaml
jobs:
  - name: local
    endpoint: dummy://localhost
```

</details>

## Troubleshooting

### Debug Mode

To troubleshoot issues with the `websphere_jmx` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m websphere_jmx
```

## Getting Logs

If you're encountering problems with the `websphere_jmx` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep websphere_jmx
```

### For non-systemd systems

```bash
sudo grep websphere_jmx /var/log/netdata/error.log
sudo grep websphere_jmx /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep websphere_jmx
```
