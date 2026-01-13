# 6.5 Trend and Capacity Alerts

Capacity planning alerts use `calc` to project when resources will be exhausted based on current usage trends.

:::tip

The examples below show simplified calc patterns for trend analysis. Stock alerts may use different time windows or thresholds. These examples demonstrate the calculation approach for capacity planning.

:::

## 6.5.1 Disk Days Remaining

```conf
template: out_of_disk_space_time
    on: disk.space
lookup: average -1h percentage of avail
     every: 1m
   calc: ($this > 0) ? ($avail / ($this / 100 * 86400 / 3600)) : 0
       warn: $this > 0 and $this < 72
       crit: $this > 0 and $this < 24
         to: sysadmin
```

## 6.5.2 Memory Leak Detection

```conf
template: ram_in_use
    on: system.ram
lookup: average -1h of used
     every: 1m
   calc: $this - $this(-1h)
       units: MB
       warn: $this > 500
       crit: $this > 1000
         to: sysadmin
```

## 6.5.3 Network Traffic Rate of Change

```conf
template: interface_inbound_errors
    on: net.errors
lookup: average -1m of inbound
     every: 1m
   calc: abs($this - $this(-5m))
       units: errors
       warn: $this > 10
       crit: $this > 100
         to: ops-team
```

## 6.5.4 Related Sections

- **[12.5 SLIs and SLOs](../best-practices/5-sli-slo-alerts.md)** - Connecting to business objectives
- **[13.1 Evaluation Architecture](../architecture/1-evaluation-architecture.md)** - Understanding how alerts compute values

## What's Next

- **[Chapter 7: Troubleshooting Alerts](../troubleshooting-alerts/index.md)** - Debugging alert issues
- **[Chapter 5: Receiving Notifications](../receiving-notifications/index.md)** - Configure alert delivery