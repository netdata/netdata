# 6.2 Service and Availability Alerts

These templates monitor service reachability and collector health using the `portcheck` and `httpcheck` contexts.

:::tip

The examples below show simplified alert configurations. Stock alerts include additional metadata fields and conditional thresholds. These examples highlight the essential parameters for quick implementation.

:::

## 6.2.1 TCP Port Unreachable

```conf
template: portcheck_connection_fails
    on: portcheck.status
lookup: average -5m unaligned percentage of no_connection,failed
     every: 10s
       crit: $this >= 40
      delay: down 5m multiplier 1.5 max 1h
         to: ops-team
```

## 6.2.2 HTTP Endpoint Health

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

## 6.2.3 Stale Collector Alert

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

## 6.2.4 Related Sections

- **[6.3 Application Alerts](3-application-alerts.md)** - Database and web server examples
- **[6.4 Anomaly-Based Alerts](4-anomaly-alerts.md)** - ML-driven detection
- **[6.5 Trend and Capacity Alerts](5-trend-capacity.md)** - Predictive capacity

## What's Next

- **[Chapter 7: Troubleshooting Alerts](../troubleshooting-alerts/index.md)** - Debugging alert issues
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Configure alert delivery