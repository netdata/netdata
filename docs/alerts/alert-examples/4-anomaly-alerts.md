# 6.4 Anomaly-Based Alerts

## 6.4.1 Anomaly Bit Alert

```conf
template: cpu_anomaly_detected
    on: system.cpu
lookup: average -5m of user
    units: %
    every: 1m
     warn: $anomaly_bit > 0.5
       to: sysadmin
```

Requires ML enabled with sufficient historical data.

## 6.4.2 Adaptive Threshold

```conf
template: traffic_anomaly
    on: net.net
lookup: average -5m of received
    units: Mbps
    every: 1m
     warn: ($this > ($this - 10m + 2 * $stddev)) && ($this > 100)
       to: netops
```

See **3.3 Calculations and Transformations** for complex expressions.

## 6.4.3 Related Sections

- **6.5 Trend and Capacity Alerts** - Predictive alerts
- **8.4 Custom Actions** - Automation with exec