# 6.5 Trend and Capacity Alerts

Capacity planning alerts use `calc` to project when resources will be exhausted based on current usage trends.

:::tip

The examples below show simplified calc patterns for trend analysis. Stock alerts may use different time windows or thresholds. These examples demonstrate the calculation approach for capacity planning.

:::

## 6.5.1 Disk Days Remaining

Capacity planning for disk space requires two coordinated templates: one to calculate the fill rate, and another to derive remaining time.

```conf
# Template 1: Calculate the disk fill rate (GB/hour)
# This is a calculation-only template used by the second template
template: disk_fill_rate
    on: disk.space
lookup: min -10m at -50m unaligned of avail
     every: 1m
   calc: ($this - $avail) / (($now - $after) / 3600)
        units: GB/hour

# Template 2: Calculate hours remaining based on fill rate
template: out_of_disk_space_time
    on: disk.space
lookup: min -10m at -50m unaligned of avail
     every: 1m
   calc: ($disk_fill_rate > 0) ? ($avail / $disk_fill_rate) : (inf)
        units: hours
        warn: $this > 0 and $this < 48
        crit: $this > 0 and $this < 24
          to: sysadmin
```

**How it works:**
1. `disk_fill_rate` calculates fill rate from historical data (`$this - $avail`) divided by time delta
2. `out_of_disk_space_time` divides available bytes by fill rate to get hours remaining
3. If fill rate is ≤ 0 (disk growing or stable), returns `inf` (never fills)

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