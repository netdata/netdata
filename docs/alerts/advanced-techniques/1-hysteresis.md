# 8.1 Hysteresis and Status-Based Conditions

## 8.1.1 The Problem with Simple Thresholds

```conf
warn: $this > 80
```

This causes flapping when CPU hovers around 80%.

## 8.1.2 Hysteresis Solution

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -10m unaligned of user,system
     units: %
     every: 1m
      warn: ($status == $CLEAR && $this > 80) || ($status >= $WARNING && $this > 70)
      crit: ($this > 95) && ($status == $CLEAR || $this > 98)
        ok: $this < 70
```

This implements asymmetric hysteresis:
- Enter WARNING at 80%, but stay WARNING until > 85%
- Enter CRITICAL at 95%, but stay CRITICAL until > 98%
- Clear only when below 70%

The `$status` variable enables different thresholds based on current state.

## 8.1.3 Related Sections

- **[4.4 Reducing Flapping](../controlling-alerts-noise/4-reducing-flapping.md)** for delay techniques
- **[7.3 Alert Flapping](../troubleshooting-alerts/flapping.md)** for debugging

## What's Next

- **[8.2 Multi-Dimensional Alerts](2-multi-dimensional.md)** for dimension and instance-based targeting
- **[8.3 Host and Label Targeting](3-label-targeting.md)** for fine-grained scoping with labels