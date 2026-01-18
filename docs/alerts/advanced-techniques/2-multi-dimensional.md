# 8.2 Multi-Dimensional and Per-Instance Alerts

Multi-dimensional alerts target specific dimensions within a chart, while templates create alerts for every matching chart instance automatically.

## 8.2.1 Dimension Selection

Target specific dimensions within a chart:

```conf
template: interface_inbound_errors
    on: net.errors
lookup: sum -10m unaligned absolute of inbound
     units: errors
     every: 1m
      warn: $this >= 5
```

This monitors only the `inbound` dimension of net.errors charts.

## 8.2.2 Scaling to All Instances

Templates automatically create one alert per matching chart.

```conf
template: interface_inbound_errors
    on: net.errors
lookup: average -5m of inbound
     every: 1m
      warn: $this > 10
```

Creates alerts for: `interface_inbound_errors-eth0`, `interface_inbound_errors-eth1`, etc.

## 8.2.3 Related Sections

- **[1.2 Alert Types: alarm vs template](../understanding-alerts/2-alert-types-alarm-vs-template.md)** for conceptual understanding

## What's Next

- **[8.3 Host and Label Targeting](3-label-targeting.md)** for fine-grained scoping with labels
- **[8.4 Custom Actions](4-custom-actions.md)** for exec-based automation