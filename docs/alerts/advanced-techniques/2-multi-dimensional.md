# 8.2 Multi-Dimensional and Per-Instance Alerts

## 8.2.1 Dimension Selection

```conf
template: network_errors
    on: net.errors
lookup: average -1m of inbound,outbound
    every: 1m
     warn: $this > 10
```

## 8.2.2 Scaling to All Instances

Templates automatically create one alert per matching chart.

```conf
template: interface_errors
    on: net.errors
lookup: average -5m of inbound
    every: 1m
     warn: $this > 10
```

Creates alerts for: `interface_errors-eth0`, `interface_errors-eth1`, etc.

## 8.2.3 Related Sections

- **1.2 Alert Types: alarm vs template** for conceptual understanding