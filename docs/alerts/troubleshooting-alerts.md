# 7. Troubleshooting Alert Behaviour

This chapter provides systematic approaches to diagnose and fix alerts that don't behave as expected. Before diving in, understand the two axes of alert problems:

| Problem Axis | Description | See Section |
|--------------|-------------|-------------|
| **Evaluation** | Alert doesn't trigger when it should | 7.1-7.3 |
| **Notification** | Alert evaluates but no notification sent | 7.5 |

## What This Chapter Covers

| Section | Problem | Diagnostic Steps |
|---------|---------|------------------|
| **7.1 Alert Never Triggers** | Alert should fire but stays CLEAR | Chart mismatch, missing data |
| **7.2 Always Critical/Warning** | Alert stuck in non-CLEAR state | Calculation errors, unit mismatches |
| **7.3 Alert Flapping** | Rapid status changes | Fix with delays, hysteresis |
| **7.4 Variables Not Found** | Expression errors, missing context | Debug variable usage |
| **7.5 Notifications Not Sent** | Alert fires but no notification | Notification chain debugging |

## 7.1 Alert Never Triggers

The alert stays `CLEAR` even when you expect it to fire.

### 7.1.1 Checklist

Work through these steps in order:

1. **Verify the alert is loaded**
   ```bash
   curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms | keys[]' | grep "your_alert_name"
   ```

2. **Check the current value**
   ```bash
   curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name'
   ```
   Look at `value`, `status`, and `updated_at`.

3. **Verify the chart exists**
   ```bash
   curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | {id, context}' | grep -A2 "your_context"
   ```

4. **Manually evaluate the condition**
   ```bash
   # Get the lookup expression from your alert and test manually
   curl -s "http://localhost:19999/api/v1/charts/system.cpu?after=-300&before=0"
   ```

### 7.1.2 Common Causes

| Cause | Symptom | Fix |
|-------|---------|-----|
| **Wrong chart/context** | Alert loads but value never changes | Use `jq` to find exact chart name |
| **Missing dimension** | Dimension doesn't exist in chart | Use `curl /api/v1/charts` to list dimensions |
| **Lookup window too short** | Alert uses 10s but metric updates slower | Increase `every` or lookup window |
| **Threshold never met** | Conditions too strict | Verify expected values are achievable |
| **Health disabled** | No alerts evaluate | Check `netdata.conf` → `[health] enabled = yes` |
| **Syntax error** | Alert fails to load | Check `/var/log/netdata/error.log` |

### 7.1.3 Finding the Right Chart or Context

**Method 1: API exploration**

```bash
# List all charts
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts | keys[]'

# Filter by context
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | select(.context == "disk.space") | {id, name, context}'

# Show dimensions for a chart
curl -s "http://localhost:19999/api/v1/charts/system.cpu" | jq '.dimensions'
```

**Method 2: Dashboard inspection**

1. Open local dashboard: `http://your-node-ip:19999`
2. Hover over chart title
3. Note the context (e.g., `system.cpu`, `disk.space`)
4. Check the dimension names in the chart menu

## 7.2 Alert Always Critical or Warning

The alert never returns to `CLEAR` once triggered.

### 7.2.1 Common Causes

| Cause | Example |
|-------|---------|
| **Threshold unit mismatch** | Alert checks `> 80` but metric is in KB/s |
| **Calculation error** | `calc` expression always evaluates true |
| **Variable typo** | `$thiss` instead of `$this` |
| **Logic error** | `warn: $this > 0 && $this < 100` but want OR |

### 7.2.2 Diagnostic Steps

**Step 1: Check the computed value**

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name'
```

Look at `value` and `exec` fields.

**Step 2: Test the expression manually**

If your alert uses:

```conf
warn: $this > 80
```

Verify:
```bash
# Get current value
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name.value'

# Is it > 80? If not, that's why CLEAR stays
```

**Step 3: Check for unit mismatches**

```conf
# WRONG: Threshold expects %, value is in MB
warn: $this < 20  # But lookup returns MB, not percentage

# RIGHT: Convert units or use correct lookup
lookup: percentage of avail  # Returns %
```

### 7.2.3 Fixing Calculation Errors

**Common `calc` mistakes:**

```conf
# WRONG: Division by zero possible
calc: $this / ($var - $var2)

# RIGHT: Handle edge cases
calc: ($this / ($var - $var2)) * ($var > $var2 ? 1 : 0)
```

See **3.3 Calculations and Transformations** for safe patterns.

## 7.3 Alert Flapping

The alert rapidly switches between statuses (CLEAR → WARNING → CRITICAL → CLEAR → WARNING).

### 7.3.1 Symptoms

- Many notification spam in a short time
- Alert status changes every evaluation cycle
- Alert shows WARNING/CRITICAL briefly, then returns to CLEAR

### 7.3.2 Solution: Add Delays

```conf
template: cpu_usage
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: $this > 80
     crit: $this > 95
   delay: up 5m down 2m    # <-- Add this
```

This requires:
- **5 minutes** above threshold before WARNING/CRITICAL fires
- **2 minutes** below threshold before returning to CLEAR

### 7.3.3 Solution: Use Smoothing Windows

Instead of short windows that catch momentary spikes:

```conf
# Flapping cause: 10s window catches momentary spikes
lookup: average -10s of user

# Better: 5-minute average smooths spikes
lookup: average -5m of user
```

### 7.3.4 Solution: Add Repeat Intervals

Limit notification frequency:

```conf
template: cpu_usage
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: $this > 80
     crit: $this > 95
   delay: up 5m down 2m
  repeat: 6h    # <-- Only notify every 6 hours for sustained issues
```

## 7.4 Variables or Metrics Not Found

The alert fails to load or shows errors about missing variables.

### 7.4.1 Debugging Variables

Use the **alarm_variables API** to see what variables are available:

```bash
# List all variables for a chart
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu"
```

Or query a specific alarm:

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms.your_alert_name'
```

Look at the `source` field for the exact variable names.

### 7.4.2 Common Variable Errors

| Error | Cause | Fix |
|-------|-------|-----|
| `unknown variable 'thiss'` | Typo: `$thiss` instead of `$this` | Correct the variable name |
| `no dimension 'usr'` | Wrong dimension name | Use `curl /api/v1/charts` to list actual dimensions |
| `$this is NaN` | Lookup returned no data | Check data availability for chart |
| `division by zero` | Expression divides by zero | Add conditional logic |

### 7.4.3 Context vs Chart Variables

- **Chart variables** (`$chart`, `$host`) are always available
- **Dimension variables** (`$user`, `$system`) require the dimension to exist
- **Context variables** are chart-specific

```conf
# Available on all charts
warn: $this > 80
warn: $chart == "system.cpu"

# Requires dimension 'user' to exist
warn: $user > 80

# Available on specific contexts only
warn: $read > $write  # Only on disk.io
```

## 7.5 Notifications Not Being Sent

The alert fires (changes status) but no one receives notifications.

### 7.5.1 Is It an Evaluation or Notification Problem?

```bash
# Check alert status
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alerts.your_alert_name'
```

- **Status changes?** Alert is evaluating correctly → Notification problem
- **Status never changes?** Alert isn't evaluating → Evaluation problem

### 7.5.2 Notification Debugging Checklist

1. **Is the alert enabled for notifications?**
   ```conf
   # Should NOT have 'to: silent' or 'enabled: no'
   template: your_alert
       to: sysadmin  # <-- Recipient defined
   ```

2. **Is the recipient configured correctly?**
   ```bash
   sudo less /etc/netdata/health_alarm_notify.conf
   # Check SEND_SLACK="YES" and webhooks configured
   ```

3. **Check notification logs**
   ```bash
   sudo tail -n 200 /var/log/netdata/error.log | grep -i notification
   ```

4. **Test the notification channel**
   ```bash
   # Send test notification
   /usr/lib/netdata/netdata-ui send-test-notification --type slack
   ```

### 7.5.3 Common Notification Issues

| Issue | Cause | Fix |
|-------|-------|-----|
| Silent recipient | `to: silent` prevents notifications | Add actual recipient |
| Channel disabled | `SEND_SLACK="NO"` in config | Enable the channel |
| Wrong severity | Only CRITICAL sent but alert is WARNING | Lower severity threshold or configure WARNING routing |
| Cloud silencing | Silencing rule matches | Check Cloud silencing rules |
| Rate limiting | Too many notifications suppressed | Add `repeat:` interval |

## 7.6 API for Debugging

Use these API calls to diagnose issues:

| API | Purpose |
|-----|---------|
| `/api/v1/alarms` | List all loaded alerts and their status |
| `/api/v1/alarms?all` | Include disabled alerts |
| `/api/v1/charts` | Find charts and contexts |
| `/api/v1/charts/{chart_id}` | Get chart dimensions and metadata |
| `/api/v1/alarm_variables` | See available variables for a chart |
| `/api/v1/health?cmd=reload` | Reload configuration |

## Key Takeaway

Most alert problems fall into two categories: **evaluation issues** (alert never fires or always fires) or **notification issues** (fires but no alerts sent). Use the systematic checklists in this chapter to diagnose and fix issues quickly.

## What's Next

- **Chapter 8: Advanced Alert Techniques** Hysteresis, multi-dimensional alerts, custom scripts
- **Chapter 9: APIs for Alerts and Events** Complete API documentation
- **Chapter 5: Receiving Notifications** Notification configuration and troubleshooting