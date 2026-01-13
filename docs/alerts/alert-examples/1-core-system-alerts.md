# 6.1 Core System Alerts

These templates demonstrate the most common alerts for fundamental server resources. Each example targets the `system.` contexts that exist on every Netdata node.

:::note

The examples below use real stock alert templates with custom thresholds for demonstration. You can use these directly or adapt the thresholds for your environment.

:::

## 6.1.1 CPU Utilization Alert

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -5m unaligned of user,system,softirq,irq,guest
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

- **6.2 Service and Availability** - Service health alerts
- **6.3 Application Alerts** - Database and web server alerts