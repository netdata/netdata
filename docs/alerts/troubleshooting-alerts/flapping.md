# 7.3 Alert Flapping

Rapid status transitions between CLEAR, WARNING, and CRITICAL create notification noise and erode trust in alerts.

## 7.3.1 Symptoms

- Many notifications in short time
- Status changes every evaluation cycle
- Alert shows WARNING/CRITICAL briefly, then CLEAR

## 7.3.2 Solution: Add Delays

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -10m unaligned of user,system
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 95
    delay: up 5m down 5m multiplier 1.5 max 1h
```

This requires the condition to stay above/below threshold for 5 minutes (with 1.5x multiplier, up to 1h max) before status changes.

## 7.3.3 Solution: Repeat Intervals

```conf
# Only notify every 6 hours for sustained WARNING issues
repeat: warning 6h critical 1h
```

The `repeat:` line controls how often notifications are sent for sustained conditions. Without this, notifications fire every evaluation cycle.

## 7.3.4 Related Sections

- **[4.4 Reducing Flapping and Noise](../controlling-alerts-noise/4-reducing-flapping.md)** for delay techniques
- **[8.1 Hysteresis and Status-Based Conditions](../advanced-techniques/1-hysteresis.md)** for status-based thresholds