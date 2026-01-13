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
lookup: average -5m of user,system
     every: 1m
      warn: $this > 80
      crit: $this > 95
    delay: up 5m down 2m
```

This requires 5 minutes above threshold before WARNING fires.

## 7.3.3 Solution: Repeat Intervals

```conf
repeat: 6h  # Only notify every 6 hours for sustained issues
```

## 7.3.4 Related Sections

- **4.4 Reducing Flapping and Noise** for delay techniques
- **8.1 Hysteresis** for status-based thresholds