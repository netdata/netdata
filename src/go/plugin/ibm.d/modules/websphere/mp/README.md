# IBM WebSphere MicroProfile collector

## Overview

Collects JVM, vendor, and REST endpoint metrics from WebSphere Liberty / Open Liberty
servers via the MicroProfile Metrics (Prometheus/OpenMetrics) endpoint.


This collector is part of the [Netdata](https://github.com/netdata/netdata) monitoring solution.

## Collected metrics

Metrics grouped by scope.

The scope defines the instance that the metric belongs to. An instance is uniquely identified by a set of labels.

### Per IBM WebSphere MicroProfile instance


These metrics refer to the entire monitored IBM WebSphere MicroProfile instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_mp.cpu_usage | process, utilization | percentage |
| websphere_mp.cpu_time | total | seconds |

These metrics refer to the entire monitored IBM WebSphere MicroProfile instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_mp.jvm_memory_heap_usage | used, free | bytes |
| websphere_mp.jvm_memory_heap_committed | committed | bytes |
| websphere_mp.jvm_memory_heap_max | limit | bytes |
| websphere_mp.jvm_heap_utilization | utilization | percentage |
| websphere_mp.jvm_gc_collections | rate | collections/s |
| websphere_mp.jvm_gc_time | total, per_cycle | milliseconds |
| websphere_mp.jvm_threads_current | daemon, other | threads |
| websphere_mp.jvm_threads_peak | peak | threads |

These metrics refer to the entire monitored IBM WebSphere MicroProfile instance.

This scope has no labels.

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_mp.threadpool_usage | active, idle | threads |
| websphere_mp.threadpool_size | size | threads |



### Per restendpoint

These metrics refer to individual restendpoint instances.

Labels:

| Label | Description |
|:------|:------------|
| method | Method identifier |
| endpoint | Endpoint identifier |

Metrics:

| Metric | Dimensions | Unit |
|:-------|:-----------|:-----|
| websphere_mp.rest_requests | requests | requests/s |
| websphere_mp.rest_response_time | average | milliseconds |


## Configuration

### File

The configuration file name for this integration is `ibm.d/websphere_mp.conf`.

You can edit the configuration file using the `edit-config` script from the
Netdata [config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration.md#the-netdata-config-directory).

```bash
cd /etc/netdata 2>/dev/null || cd /opt/netdata/etc/netdata
sudo ./edit-config ibm.d/websphere_mp.conf
```

### Options

The following options can be defined globally or per job.

| Name | Description | Default | Required | Min | Max |
|:-----|:------------|:--------|:---------|:----|:----|
| tls_key | Client key path | `` | no | - | - |
| tls_cert | Client certificate path | `` | no | - | - |
| tls_ca | Custom CA bundle path | `` | no | - | - |
| tls_skip_verify | Skip TLS certificate verification | `false` | no | - | - |
| headers | Custom headers | `<no value>` | no | - | - |
| proxy_password | Proxy password | `` | no | - | - |
| proxy_username | Proxy username | `` | no | - | - |
| proxy_url | Proxy URL | `` | no | - | - |
| not_follow_redirects | Disable HTTP redirects | `false` | no | - | - |
| timeout | Request timeout in seconds | `10000000000` | no | - | - |
| password | Password for authentication | `` | no | - | - |
| username | Username for authentication | `` | no | - | - |
| url | Target URL | `` | no | - | - |
| update_every | Data collection frequency | `1` | no | 1 | - |
| Vnode | Vnode | `` | no | - | - |
| CellName | CellName appends the Liberty cell label to every exported time-series. | `` | no | - | - |
| NodeName | NodeName appends the Liberty node label to every exported time-series. | `` | no | - | - |
| ServerName | ServerName appends the Liberty server label to every exported time-series. | `` | no | - | - |
| MetricsEndpoint | MetricsEndpoint overrides the metrics path relative to the base URL (accepts absolute URLs as well). | `/metrics` | no | - | - |
| CollectJVMMetrics | CollectJVMMetrics toggles JVM/base scope metrics scraped from the MicroProfile endpoint. | `enabled` | no | - | - |
| CollectRESTMetrics | CollectRESTMetrics toggles per-endpoint REST/JAX-RS metrics (may introduce cardinality). | `enabled` | no | - | - |
| MaxRESTEndpoints | MaxRESTEndpoints limits how many REST endpoints are exported when REST metrics are enabled (0 disables the limit). | `50` | no | - | - |
| CollectRESTMatching | CollectRESTMatching filters REST endpoints using glob-style patterns (supports `*`, `?`, `!` prefixes). | `` | no | - | - |

### Examples

#### Basic configuration

IBM WebSphere MicroProfile monitoring with default settings.

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

To troubleshoot issues with the `websphere_mp` collector, run the `ibm.d.plugin` with the debug option enabled.
The output should give you clues as to why the collector isn't working.

- Navigate to the `plugins.d` directory, usually at `/usr/libexec/netdata/plugins.d/`
- Switch to the `netdata` user
- Run the `ibm.d.plugin` to debug the collector:

```bash
sudo -u netdata ./ibm.d.plugin -d -m websphere_mp
```

## Getting Logs

If you're encountering problems with the `websphere_mp` collector, follow these steps to retrieve logs and identify potential issues:

- **Run the command** specific to your system (systemd, non-systemd, or Docker container).
- **Examine the output** for any warnings or error messages that might indicate issues. These messages will typically provide clues about the root cause of the problem.

### For systemd systems (most Linux distributions)

```bash
sudo journalctl -u netdata --reverse | grep websphere_mp
```

### For non-systemd systems

```bash
sudo grep websphere_mp /var/log/netdata/error.log
sudo grep websphere_mp /var/log/netdata/collector.log
```

### For Docker containers

```bash
sudo docker logs netdata 2>&1 | grep websphere_mp
```
