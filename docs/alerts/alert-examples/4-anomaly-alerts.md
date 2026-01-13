# 6.4 Anomaly-Based Alerts

Anomaly detection uses Netdata's ML models to identify unusual metric behavior without fixed thresholds. Requires `ml.conf` enabled on the node.

## 6.4.1 Anomaly Bit Alert

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

## 6.4.2 Adaptive Threshold

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

## 6.4.3 Related Sections

- **6.5 Trend and Capacity Alerts** - Predictive alerts
- **8.4 Custom Actions** - Automation with exec