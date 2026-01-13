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
lookup: average -5m of user,system
     every: 1m
      warn: ($this > 80) && ($status != $WARNING)
      crit: ($this > 95) && ($status != $CRITICAL)
```

Behavior:
- Enter WARNING at 80%, but only clear when below 70%
- Enter CRITICAL at 95%, but only clear when below 85%

## 8.1.3 Related Sections

- **4.4 Reducing Flapping** for delay techniques
- **7.3 Alert Flapping** for debugging