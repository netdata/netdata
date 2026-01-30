# 6. Alert Examples and Common Patterns

This chapter provides **practical alert templates** you can adapt for common monitoring scenarios.

:::tip

These examples follow the structure established in **[2.1 Quick Start: Create Your First Alert](/docs/alerts/creating-alerts-pages/1-quick-start-create-your-first-alert.md)**. Use the Cloud UI or configuration files as preferred.

:::

## Here

| Section | Examples |
|---------|----------|
| **[6.1 Core System Alerts](#61-core-system-alerts)** | CPU, RAM, disk, network—system resource monitoring |
| **[6.2 Service and Availability](#62-service-and-availability)** | Service health, stale collectors, HTTP checks |
| **[6.3 Application Alerts](#63-application-alerts)** | Databases, web servers, caches, message queues |
| **[6.4 Anomaly-Based Alerts](#64-anomaly-based-alerts)** | ML-driven detection using anomaly-bit |
| **[6.5 Trend and Capacity Alerts](#65-trend-and-capacity)** | Capacity planning, growth rate projections |

## 6.1 Core System Alerts

These templates demonstrate the most common alerts for fundamental server resources. Each example targets the `system.` contexts that exist on every Netdata node.

:::tip

The examples below use simplified syntax based on real stock alert templates. Stock alerts include additional fields like `class`, `type`, `component`, `delay`, and conditional thresholds. These examples show the essential fields for quick reference—you can copy, modify, and extend them for your needs.

:::

### 6.1.1 CPU Utilization Alert

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -10m unaligned of user,system,softirq,irq,guest
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 95
        to: sysadmin
      info: CPU utilization at ${value}%
```

### 6.1.2 Memory Pressure Alert

```conf
template: ram_available
    on: mem.available
lookup: average -5m of avail
     units: MB
     every: 1m
      warn: $this < 1024
      crit: $this < 512
        to: sysadmin
```

### 6.1.3 Disk Space Alert

```conf
template: disk_space_usage
    on: disk.space
lookup: average -1m percentage of used
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 90
        to: sysadmin
```

### 6.1.4 Network Errors Alert

```conf
template: interface_inbound_errors
    on: net.errors
lookup: average -5m of inbound
     units: errors
     every: 1m
      warn: $this > 0
      crit: $this > 10
        to: sysadmin
```

## 6.2 Service and Availability Alerts

These templates monitor service reachability and collector health using the `portcheck` and `httpcheck` contexts.

:::tip

The examples below show simplified alert configurations. Stock alerts include additional metadata fields and conditional thresholds. These examples highlight the essential parameters for quick implementation.

:::

### 6.2.1 TCP Port Unreachable

```conf
template: portcheck_connection_fails
    on: portcheck.status
lookup: average -5m unaligned percentage of no_connection,failed
     every: 10s
       crit: $this >= 40
      delay: down 5m multiplier 1.5 max 1h
         to: ops-team
```

### 6.2.2 HTTP Endpoint Health

```conf
template: httpcheck_web_service_bad_status
    on: httpcheck.status
lookup: average -5m unaligned percentage of bad_status
     every: 10s
       warn: $this >= 10 AND $this < 40
       crit: $this >= 40
      delay: down 5m multiplier 1.5 max 1h
         to: ops-team
```

### 6.2.3 Stale Collector Alert

```conf
template: plugin_availability_status
    on: netdata.plugin_availability_status
     calc: $now - $last_collected_t
     units: seconds ago
     every: 1m
      warn: $this > 300
      crit: $this > 600
         to: ops-team
```

## 6.3 Application-Level Alerts

These templates demonstrate application-specific monitoring using contexts provided by database and web server collectors.

:::tip

To see more application alerts, browse the available collectors in the documentation.

:::

### 6.3.1 MySQL Alerts

Slow query detection for MySQL and MariaDB databases.

```conf
template: mysql_10s_slow_queries
    on: mysql.queries
lookup: sum -10s of slow_queries
     units: slow queries
     every: 10s
       warn: $this > 5
       crit: $this > 10
      delay: down 5m multiplier 1.5 max 1h
         to: dba-team
```

### 6.3.2 PostgreSQL Alerts

Deadlock detection for PostgreSQL databases.

```conf
template: postgres_db_deadlocks_rate
    on: postgres.db_deadlocks_rate
lookup: average -5m of deadlocks
     every: 1m
      warn: $this > 0
      crit: $this > 5
        to: dba-team
```

### 6.3.3 Redis Alerts

Connection rejection detection for Redis.

```conf
template: redis_connections_rejected
    on: redis.connections
lookup: sum -1m unaligned of rejected
     every: 10s
      warn: $this > 0
        to: cache-team
```

## 6.4 Anomaly-Based Alerts

Anomaly detection uses Netdata's ML models to identify unusual metric behavior without fixed thresholds. Requires `ml.conf` enabled on the node.

:::tip

The examples below show conceptual patterns for anomaly-based alerting. Stock alerts include additional metadata and conditional logic. Use these as starting points for your own anomaly detection configurations.

:::

### 6.4.1 Anomaly Bit Alert

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -5m unaligned of user
     units: %
     every: 1m
      warn: $anomaly_bit > 0.5
        to: sysadmin
```

Requires ML enabled with sufficient historical data.

### 6.4.2 Adaptive Threshold

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -5m of user
     units: %
     every: 1m
      warn: ($this > ($this - 10m + 2 * $stddev)) && ($this > 80)
        to: netops
```

See **3.3 Calculations and Transformations** for complex expressions.

## 6.5 Trend and Capacity Alerts

Capacity planning alerts use `calc` to project when resources will be exhausted based on current usage trends.

:::tip

The examples below show simplified calc patterns for trend analysis. Stock alerts may use different time windows or thresholds. These examples demonstrate the calculation approach for capacity planning.

:::

### 6.5.1 Disk Days Remaining

Capacity planning for disk space requires two coordinated templates: one to calculate the fill rate, and another to derive remaining time.

```conf
# Template 1: Calculate the disk fill rate (GB/hour)
# This is a calculation-only template used by the second template
template: disk_fill_rate
    on: disk.space
lookup: min -10m at -50m unaligned of avail
     every: 1m
   calc: ($this - $avail) / (($now - $after) / 3600)
        units: GB/hour

# Template 2: Calculate hours remaining based on fill rate
template: out_of_disk_space_time
    on: disk.space
lookup: min -10m at -50m unaligned of avail
     every: 1m
   calc: ($disk_fill_rate > 0) ? ($avail / $disk_fill_rate) : (inf)
        units: hours
        warn: $this > 0 and $this < 48
        crit: $this > 0 and $this < 24
          to: sysadmin
```

**How it works:**
1. `disk_fill_rate` calculates fill rate from historical data (`$this - $avail`) divided by time delta
2. `out_of_disk_space_time` divides available bytes by fill rate to get hours remaining
3. If fill rate is ≤ 0 (disk growing or stable), returns `inf` (never fills)

### 6.5.2 Memory Leak Detection

```conf
template: ram_in_use
    on: system.ram
lookup: average -1h of used
     every: 1m
   calc: $this - $this(-1h)
       units: MB
       warn: $this > 500
       crit: $this > 1000
         to: sysadmin
```

### 6.5.3 Network Traffic Rate of Change

```conf
template: interface_inbound_errors
    on: net.errors
lookup: average -1m of inbound
     every: 1m
   calc: abs($this - $this(-5m))
       units: errors
       warn: $this > 10
       crit: $this > 100
         to: ops-team
```

## Related Sections

- **[2. Creating and Managing Alerts](/docs/alerts/creating-alerts-pages/README.md)** - Creating and editing alerts via configuration files
- **[7. Troubleshooting Alerts](/docs/alerts/troubleshooting-alerts/README.md)** - Debugging alert issues
- **[5. Receiving Notifications](/docs/alerts/receiving-notifications/README.md)** - Configure alert delivery
- **[12. Best Practices for Alerting](/docs/alerts/best-practices/README.md)** - Best practices for designing effective alerts