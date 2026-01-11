# 6. Alert Examples and Common Patterns

This chapter provides **practical alert templates** you can adapt for common monitoring scenarios. Each example includes the full alert definition, explains what it monitors, and suggests customizations for your environment.

:::tip
These examples follow the structure established in **2.1 Quick Start: Create Your First Alert**. Use the Cloud UI or configuration files as preferred.
:::

## What This Chapter Covers

| Section | Examples |
|---------|----------|
| **6.1 Core System Alerts** | CPU, RAM, disk, network—system resource monitoring |
| **6.2 Service Availability** | Service health, stale collectors, HTTP checks |
| **6.3 Application Alerts** | Databases, web servers, caches, message queues |
| **6.4 Anomaly-Based Alerts** | ML-driven detection using anomaly-bit |
| **6.5 Trend and Capacity Alerts** | Capacity planning, growth rate projections |

## 6.1 Core System Alerts

These alerts monitor fundamental system resources that affect all servers.

### 6.1.1 CPU Utilization Alert

```conf
# Alert when CPU usage exceeds thresholds
template: cpu_high_usage
    on: system.cpu
lookup: average -5m of user,system,softirq,irq,guest
    units: %
    every: 1m
     warn: $this > 80
     crit: $this > 95
       to: sysadmin
     info: CPU utilization at ${value}%, see cpu usage per core in system.cpu
```

**Customization options:**

| Variation | Change |
|-----------|--------|
| Different time window | `lookup: average -10m` (slower fluctuations) |
| Per-core alerting | Use `template` instead of `alarm` to alert on each core |
| Idle-time warning | `warn: $this < 10` (unusually idle) |

### 6.1.2 Memory Pressure Alert

```conf
# Alert when available memory drops below threshold
template: ram_low_available
    on: system.ram
lookup: average -5m of available
    units: MB
    every: 1m
     warn: $this < 1024
     crit: $this < 512
       to: sysadmin
     info: Available RAM is ${value}MB, total RAM is ${total:ram}MB
```

**For systems with swap:**

```conf
# Alert when swap usage is high
template: swap_high_usage
    on: system.swap
lookup: average -5m of used
    units: MB
    every: 1m
     warn: $this > 1024
     crit: $this > 2048
       to: sysadmin
     info: System is using ${value}MB of swap
```

### 6.1.3 Disk Space Alert

```conf
# Alert when any filesystem has less than 10% free space
template: disk_space_low
    on: disk.space
lookup: average -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 10
     crit: $this < 5
       to: sysadmin
     info: Filesystem ${chart} has ${value}% free space remaining
```

**Specific filesystem only:**

```conf
# Alert only on root filesystem
alarm: root_disk_space_low
    on: disk.space./
lookup: average -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 15
     crit: $this < 5
       to: sysadmin
```

### 6.1.4 Disk I/O Latency Alert

```conf
# Alert when disk I/O operations are slower than expected
template: disk_io_latency_high
    on: disk.io
lookup: average -5m of avgsz
    units: milliseconds
    every: 1m
     warn: $this > 20
     crit: $this > 100
       to: sysadmin
     info: Average I/O size is ${value}ms, check disk performance
```

### 6.1.5 Network Errors Alert

```conf
# Alert on network interface errors
template: net_errors_high
    on: net.net
lookup: average -5m of errors
    units: packets
    every: 1m
     warn: $this > 0
     crit: $this > 10
       to: sysadmin
     info: Interface ${chart} has ${value} errors in last 5 minutes
```

## 6.2 Service and Availability Alerts

These alerts monitor service health and availability rather than resource utilization.

### 6.2.1 Service Not Running Alert

```conf
# Alert when a critical service is not running
template: service_not_running
    on: health.service
lookup: average -1m of status
    units: status
    every: 1m
     crit: $this == 0
       to: ops-team
     info: Service ${chart} is not running
```

**Common service names:**

| Service | Chart Name |
|---------|------------|
| nginx | `health.service nginx` |
| mysql | `health.service mysql` |
| postgresql | `health.service postgresql` |
| redis | `health.service redis` |

### 6.2.2 Stale Collector Alert

```conf
# Alert when a collector hasn't updated in expected time
template: collector_stale
    on: health.collector
lookup: average -5m of age
    units: seconds
    every: 1m
     warn: $this > 300
     crit: $this > 600
       to: ops-team
     info: Collector ${chart} hasn't updated in ${value} seconds
```

This catches collectors that stopped collecting data—often indicates a process crash or connection issue.

### 6.2.3 HTTP Endpoint Health Check

```conf
# Alert when HTTP endpoint returns error
template: http_endpoint_failing
    on: health.service
lookup: average -1m of status
    units: status
    every: 1m
     warn: $this == 0
     crit: $this == 0
       to: ops-team
     info: HTTP endpoint ${chart} is not responding
```

For more advanced HTTP health checks, use the **httpcheck** collector:

```bash
# Enable httpcheck collector
sudo /etc/netdata/edit-config go.d/httpcheck.conf
```

Configure endpoint URLs and expected response codes.

### 6.2.4 TCP Port Unreachable

```conf
# Alert when TCP port is not accepting connections
template: tcp_port_closed
    on: health.service
lookup: average -1m of failed
    units: failures
    every: 1m
     crit: $this > 0
       to: ops-team
     info: TCP connection failures detected on ${chart}
```

## 6.3 Application-Level Alerts

These alerts target specific applications using their metrics.

### 6.3.1 MySQL/MariaDB Alerts

```conf
# Alert on slow queries
template: mysql_slow_queries
    on: mysql.global_status
lookup: average -5m of slow_queries
    units: queries
    every: 1m
     warn: $this > 10
     crit: $this > 100
       to: dba-team
     info: ${value} slow queries detected, check query optimization

# Alert on connection errors
template: mysql_connections_high
    on: mysql.connections
lookup: average -5m of aborted
    units: connections
    every: 1m
     warn: $this > 5
     crit: $this > 20
       to: dba-team
     info: ${value} aborted connections, check connection pooling
```

### 6.3.2 PostgreSQL Alerts

```conf
# Alert on deadlocks
template: pg_deadlocks_high
    on: pg.stat_database
lookup: average -5m of deadlocks
    units: locks
    every: 1m
     warn: $this > 0
     crit: $this > 5
       to: dba-team
     info: ${value} deadlocks detected in ${chart} database

# Alert on replication lag
template: pg_replication_lag
    on: pg.replication
lookup: average -1m of lag
    units: MB
    every: 1m
     warn: $this > 100
     crit: $this > 1000
       to: dba-team
     info: Replication lag is ${value}MB
```

### 6.3.3 Redis Alerts

```conf
# Alert on connected clients
template: redis_clients_high
    on: redis.clients
lookup: average -5m of connected
    units: clients
    every: 1m
     warn: $this > 10000
     crit: $this > 50000
       to: cache-team
     info: ${value} clients connected to Redis

# Alert on evicted keys
template: redis_evictions_high
    on: redis.statistics
lookup: average -5m of evicted
    units: keys
    every: 1m
     warn: $this > 100
     crit: $this > 1000
       to: cache-team
     info: ${value} keys evicted due to memory pressure
```

### 6.3.4 Nginx Alerts

```conf
# Alert on 5xx errors
template: nginx_errors_high
    on: nginx.requests
lookup: average -5m of 5xx
    units: requests
    every: 1m
     warn: $this > 10
     crit: $this > 100
       to: web-team
     info: ${value} 5xx errors in last 5 minutes

# Alert on request latency (if available)
template: nginx_latency_high
    on: nginx.request
lookup: average -5m of latency
    units: ms
    every: 1m
     warn: $this > 500
     crit: $this > 2000
       to: web-team
     info: Average request latency is ${value}ms
```

### 6.3.5 Apache Kafka Alerts

```conf
# Alert on under-replicated partitions
template: kafka_under_replicated
    on: kafka.replication
lookup: average -1m of under_replicated
    units: partitions
    every: 1m
     warn: $this > 0
     crit: $this > 10
       to: platform-team
     info: ${value} partitions are under-replicated

# Alert on offline partitions
template: kafka_offline_partitions
    on: kafka.replication
lookup: average -1m of offline
    units: partitions
    every: 1m
     crit: $this > 0
       to: platform-team
     info: ${value} partitions are offline
```

## 6.4 Anomaly-Based Alerts

These alerts use Netdata's ML features to detect unusual behavior without fixed thresholds.

### 6.4.1 Anomaly Bit Alert

```conf
# Alert when metric is anomalous
template: cpu_anomaly_detected
    on: system.cpu
lookup: average -5m of user
    units: %
    every: 1m
     warn: $anomaly_bit > 0.5
       to: sysadmin
     info: CPU usage is anomalous: ${value}% (anomaly score: ${anomaly_bit})
```

**Requirements:**

- Netdata ML must be enabled
- Sufficient historical data for the metric
- `anomaly_bit` variable available in the context

### 6.4.2 Adaptive Threshold Alert

For metrics with variable "normal" ranges:

```conf
# Dynamic threshold based on historical values
template: traffic_anomaly
    on: net.net
lookup: average -5m of bandwidth
    units: Mbps
    every: 1m
     warn: ($this > ($this - 10m + 2 * $stddev)) && ($this > 100)
       to: netops
     info: Traffic ${chart} is ${value}Mbps, unusually high vs recent baseline
```

See **3.3 Calculations and Transformations** for details on using `calc` for complex expressions.

## 6.5 Trend and Capacity Alerts

These alerts predict future conditions based on current trends.

### 6.5.1 Disk Filling Up Alert

```conf
# Estimate days until disk is full
template: disk_days_remaining
    on: disk.space
lookup: average -1h percentage of avail
    units: %
    every: 1m
 calc: (($this / 100) * 86400) / ($this - $this(1h)) / 86400
     warn: $this < 30
     crit: $this < 7
       to: sysadmin
     info: Estimated ${value} days until disk ${chart} is full
```

**How it works:**

- Compares current free space to free space 1 hour ago
- Calculates rate of change
- Extrapolates to predict days until full

### 6.5.2 Memory Growth Trend

```conf
# Detect memory leak patterns
template: memory_growing
    on: system.ram
lookup: average -1h of used
    units: MB
    every: 1m
 calc: ($this - $this(-1h)) / 1024
     warn: $this > 0.5
     crit: $this > 2
       to: sysadmin
     info: Memory growing at ${value}GB/hour, possible memory leak
```

### 6.5.3 Rate of Change Alert

```conf
# Alert on rapid changes (spikes or drops)
template: connection_spike
    on: net.connections
lookup: average -1m of curr
    units: connections
    every: 1m
 calc: abs($this - $this(-5m)) / $this(-5m) * 100
     warn: $this > 50
     crit: $this > 100
       to: ops-team
     info: Connection count changed by ${value}% in 5 minutes
```

## Key Takeaway

These examples cover the most common monitoring scenarios. Adapt thresholds (`warn`, `crit`), time windows (`lookup`), and recipients (`to:`) to match your environment. For custom applications, identify the relevant context and create similar templates.

## What's Next

- **Chapter 7: Troubleshooting Alert Behaviour** Debug why alerts don't fire as expected
- **Chapter 8: Advanced Alert Techniques** Multi-dimensional alerts, custom scripts, performance
- **Chapter 10: Netdata Cloud Alert Features** Cloud-specific notification and silencing features