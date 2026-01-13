# 6.1 Core System Alerts

These templates demonstrate the most common alerts for fundamental server resources. Each example targets the `system.` contexts that exist on every Netdata node.

:::tip

The examples below use simplified syntax based on real stock alert templates. Stock alerts include additional fields like `class`, `type`, `component`, `delay`, and conditional thresholds. These examples show the essential fields for quick reference—you can copy, modify, and extend them for your needs.

:::

## 6.1.1 CPU Utilization Alert

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

## 6.1.2 Memory Pressure Alert

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

## 6.1.3 Disk Space Alert

```conf
template: disk_space_usage
    on: disk.space
lookup: average -1m percentage of avail
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 90
        to: sysadmin
```

## 6.1.4 Network Errors Alert

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

## 6.1.5 Related Sections

- **[6.2 Service and Availability](2-service-availability.md)** - Service health alerts
- **[6.3 Application Alerts](3-application-alerts.md)** - Database and web server examples
- **[6.4 Anomaly-Based Alerts](4-anomaly-alerts.md)** - ML-driven detection
- **[6.5 Trend and Capacity Alerts](5-trend-capacity.md)** - Capacity planning

## What's Next

- **[Chapter 7: Troubleshooting Alerts](../troubleshooting-alerts/index.md)** - Debugging alert issues
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Configure alert delivery