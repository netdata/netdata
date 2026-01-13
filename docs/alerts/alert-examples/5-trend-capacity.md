# 6.5 Trend and Capacity Alerts

Capacity planning alerts use `calc` to project when resources will be exhausted based on current usage trends.

:::note

The examples below use `template:` names that describe their purpose. They demonstrate calculation-based alerts for capacity planning. Adapt the context and calculation for your metrics.

:::

## 6.5.1 Disk Days Remaining

```conf
template: disk_days_remaining_alert
    on: disk.space
lookup: average -1h percentage of avail
     every: 1m
  calc: (($this / 100) * 86400) / ($this - $this(1h)) / 86400
      warn: $this < 30
      crit: $this < 7
        to: sysadmin
```

## 6.5.2 Memory Leak Detection

```conf
template: memory_growth_alert
    on: system.ram
lookup: average -1h of used
     units: MB
     every: 1m
  calc: ($this - $this(-1h)) / 1024
      warn: $this > 0.5
      crit: $this > 2
        to: sysadmin
```

## 6.5.3 Rate of Change

```conf
template: connection_spike_alert
    on: net.packets
lookup: average -1m of received
     every: 1m
  calc: abs($this - $this(-5m)) / $this(-5m) * 100
      warn: $this > 50
      crit: $this > 100
        to: ops-team
```

## 6.5.4 Related Sections

- **12.5 SLIs and SLOs** - Connecting to business objectives
- **13.1 Evaluation Architecture** - Understanding how alerts compute values