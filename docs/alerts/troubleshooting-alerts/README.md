# 7. Troubleshooting Alert Behaviour

This chapter provides systematic approaches to diagnose and fix alerts that don't behave as expected.

## Here

| Section | Problem |
|---------|----------|
| **[7.1 Alert Never Triggers](#71-alert-never-triggers)** | Alert should fire but stays CLEAR |
| **[7.2 Always Critical or Warning](#72-alert-always-critical-or-warning)** | Alert stuck in non-CLEAR state |
| **[7.3 Alert Flapping](#73-alert-flapping)** | Rapid status changes between states |
| **[7.4 Variables or Metrics Not Found](#74-variables-or-metrics-not-found)** | Expression errors referencing missing variables |
| **[7.5 Notifications Not Sent](#75-notifications-not-being-sent)** | Alert fires but no notification received |

## 7.1 Alert Never Triggers

The alert stays `CLEAR` even when you expect it to fire.

### 7.1.1 Checklist

1. Verify the alert is loaded:
```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms | keys[]' | grep "your_alert"
```

2. Check current value:
```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name'
```

3. Verify the chart exists:
```bash
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | select(.context == "your_context")'
```

### 7.1.2 Common Causes

| Cause | Fix |
|-------|-----|
| Wrong chart/context | Find exact chart name via API |
| Missing dimension | List dimensions: `curl /api/v1/charts` |
| Threshold never met | Verify values are achievable |
| Health disabled | Check `netdata.conf` → `[health] enabled = yes` |

## 7.2 Alert Always Critical or Warning

The alert never returns to `CLEAR`. This typically indicates threshold issues, calculation errors, or variable problems.

### 7.2.1 Common Causes

| Cause | Example |
|-------|---------|
| Threshold unit mismatch | Alert checks `> 80` but metric is in KB/s |
| Calculation error | `calc` expression always true |
| Variable typo | `$thiss` instead of `$this` |

### 7.2.2 Diagnostic Steps

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name.value'
```

Check if the value actually crosses the threshold.

### 7.2.3 Fixing Calculation Errors

```conf
# WRONG: Division by zero possible
calc: $this / ($var - $var2)

# RIGHT: Handle edge cases
calc: ($this / ($var - $var2)) * ($var > $var2 ? 1 : 0)
```

See **[3.3 Calculations and Transformations](../alert-configuration-syntax/3-calculations-and-transformations.md)** for expression details.

## 7.3 Alert Flapping

Rapid status transitions between CLEAR, WARNING, and CRITICAL create notification noise and erode trust in alerts.

### 7.3.1 Symptoms

- Many notifications in short time
- Status changes every evaluation cycle
- Alert shows WARNING/CRITICAL briefly, then CLEAR

### 7.3.2 Solution: Add Delays

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

### 7.3.3 Solution: Repeat Intervals

```conf
# Only notify every 6 hours for sustained WARNING issues
repeat: warning 6h critical 1h
```

The `repeat:` line controls how often notifications are sent for sustained conditions. Without this, notifications fire every evaluation cycle.

## 7.4 Variables or Metrics Not Found

Configuration errors referencing non-existent variables or dimensions prevent alerts from loading.

### 7.4.1 Debugging Variables

```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu"
```

This shows all available variables for a chart.

### 7.4.2 Common Variable Errors

| Error | Fix |
|-------|-----|
| `unknown variable 'thiss'` | Correct to `$this` |
| `no dimension 'usr'` | Use actual dimension name from API |
| `$this is NaN` | Check data availability |

## 7.5 Notifications Not Being Sent

The alert fires but no one receives notifications.

### 7.5.1 Is It Evaluation or Notification?

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alerts.your_alert_name'
```

- Status changes? → Evaluation works, notification problem
- Status never changes? → Evaluation problem

### 7.5.2 Notification Checklist

1. Is recipient defined? `to: sysadmin` not `to: silent`
2. Is channel enabled? `SEND_SLACK=YES`
3. Check logs: `tail /var/log/netdata/health.log | grep notification`

### 7.5.3 Common Issues

| Issue | Fix |
|-------|-----|
| Silent recipient | Change `to: sysadmin` |
| Channel disabled | Enable in `health_alarm_notify.conf` |
| Wrong severity | Configure WARNING routing |

## Related Sections

- **[4.4 Reducing Flapping and Noise](../controlling-alerts-noise/4-reducing-flapping.md)** - Delay and repeat techniques
- **[5.5 Testing and Troubleshooting](../receiving-notifications/5-testing-troubleshooting.md)** - Delivery verification
- **[8.1 Hysteresis and Status-Based Conditions](../essential-patterns/README.md)** - Status-dependent thresholds