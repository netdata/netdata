# 6.1 Core System Alerts

## 6.1.1 CPU Utilization Alert

```conf
template: cpu_high_usage
    on: system.cpu
lookup: average -5m of user,system,softirq,irq,guest
    units: %
    every: 1m
     warn: $this > 80
     crit: $this > 95
       to: sysadmin
     info: CPU utilization at ${value}%
```

## 6.1.2 Memory Pressure Alert

```conf
template: ram_low_available
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
template: disk_space_low
    on: disk.space
lookup: average -1m percentage of avail
    units: %
    every: 1m
     warn: $this < 10
     crit: $this < 5
       to: sysadmin
```

## 6.1.4 Network Errors Alert

```conf
template: net_errors_high
    on: net.errors
lookup: average -5m of inbound,outbound
    units: errors
    every: 1m
     warn: $this > 0
     crit: $this > 10
       to: sysadmin
```

## 6.1.5 Related Sections

- **6.2 Service and Availability** - Service health alerts
- **6.3 Application Alerts** - Database and web server alerts