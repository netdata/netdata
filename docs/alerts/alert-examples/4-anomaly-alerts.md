# 6.4 Anomaly-Based Alerts

Anomaly detection uses Netdata's ML models to identify unusual metric behavior without fixed thresholds. Requires `ml.conf` enabled on the node.

:::tip

The examples below show conceptual patterns for anomaly-based alerting. Stock alerts include additional metadata and conditional logic. Use these as starting points for your own anomaly detection configurations.

:::

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

- **[6.5 Trend and Capacity Alerts](5-trend-capacity.md)** - Predictive alerts
- **[8.4 Custom Actions with exec](../advanced-techniques/4-custom-actions.md)** - Automation with exec

## What's Next

- **[Chapter 7: Troubleshooting Alerts](../troubleshooting-alerts/index.md)** - Debugging alert issues
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Configure alert delivery