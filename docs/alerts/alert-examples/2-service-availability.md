# 6.2 Service and Availability Alerts

These templates monitor service reachability and collector health using the `portcheck` and `httpcheck` contexts.

## 6.2.1 TCP Port Unreachable

```conf
template: portcheck_connection_fails
    on: portcheck.status
lookup: average -1m percentage of failed
     every: 1m
      crit: $this > 0
        to: ops-team
```

## 6.2.2 HTTP Endpoint Health

```conf
template: httpcheck_web_service_bad_status
    on: httpcheck.status
lookup: average -1m percentage of bad_status
     every: 1m
      warn: $this > 0
      crit: $this > 10
        to: ops-team
```

## 6.2.3 Stale Collector Alert

```conf
template: plugin_data_collection_status
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