# 4. Controlling Alerts and Noise

Now that you know **how to create alerts** (Chapter 2) and **how they work** (Chapter 1), this chapter shows you how to **control alert behavior** to reduce noise without losing important signals.

Every alerting system faces the same challenge: **alert fatigue**. When alerts fire too frequently or for conditions that don't need immediate attention, responders start to ignore them—and that's when real problems get missed.

Netdata gives you multiple layers of control:

| Layer | What It Does | When to Use |
|-------|---------------|-------------|
| **Disabling** | Stops alert evaluation entirely | You never want this alert to run |
| **Silencing** | Evaluates but suppresses notifications | You want to defer or skip notifications for a period |
| **Delays & Hysteresis** | Requires conditions to hold before changing status | Preventing flapping between states |
| **Repeat Intervals** | Limits notification frequency | Avoids notification storms for sustained conditions |

## What This Chapter Covers

| Section | What You'll Learn |
|---------|-------------------|
| **4.1 Disabling Alerts** | How to completely stop alert evaluation (globally, per-host, or per-alert) |
| **4.2 Silencing vs Disabling** | The critical difference between stopping evaluation vs. suppressing notifications |
| **4.3 Silencing in Netdata Cloud** | How to use Cloud silencing rules for space-wide or scheduled quiet periods |
| **4.4 Reducing Flapping and Noise** | Practical techniques: delays, hysteresis, repeat intervals, and smoothing |

## 4.1 Disabling Alerts

Disabling an alert stops it from being **evaluated entirely**. The health engine won't run the check, assign status, or generate events.

<details>
<summary><strong>When to Disable Instead of Silence</strong></summary>

Disable an alert when:

- **The alert is not relevant** to your environment (for example, you don't run MySQL)
- **You've replaced it** with a custom alert (same name)
- **The monitored service is retired** and the alert will never fire correctly
- **You want a permanent solution** rather than temporary silencing

For temporary quiet periods, see **4.3 Silencing in Netdata Cloud** instead.

</details>

### 4.1.1 Disable a Specific Alert

**Method 1: Via Configuration File**

Create or edit a file in `/etc/netdata/health.d/`:

```bash
sudo /etc/netdata/edit-config health.d/disabled.conf
```

Add the alert you want to disable:

```conf
# Disable stock alert that doesn't apply to our environment
alarm: mysql_gtid_binlog_gtid_0

# Disable by setting enabled to no
template: some_stock_alert
   enabled: no
```

Reload configuration:

```bash
sudo netdatacli reload-health
```

**Method 2: Disable All Alerts for a Host**

To disable **all** health checks on a specific host, set in `netdata.conf`:

```ini
[health]
    enabled = no
```

This is useful when:
- A node is being decommissioned but you want to keep it running for diagnostics
- You're doing maintenance and don't want alerts firing
- You want to completely stop health monitoring on a development or test node

### 4.1.2 Disable Evaluation While Keeping Notifications

If you want to stop the alert from **ever changing status** but still see it in the UI, set both conditions to false:

```conf
template: noisy_alert
   on: system.cpu
   warn: ($this) = 0
   crit: ($this) = 0
```

This keeps the alert loaded but ensures it never triggers notifications.

### 4.1.3 Disable via Health Management API

You can also disable alerts programmatically:

```bash
# Disable a specific alert
curl -s "http://localhost:19999/api/v1/health?cmd=disable&alarm=my_alert"

# Disable all alerts
curl -s "http://localhost:19999/api/v1/health?cmd=disable_all"
```

See **9.4 Health Management API** for full documentation.

## 4.2 Silencing Versus Disabling Alerts

This distinction is critical—confusing them is one of the most common sources of alert configuration mistakes.

| Aspect | Disabling | Silencing |
|--------|-----------|-----------|
| **What stops** | Alert **evaluation** (never runs) | **Notifications** only (evaluation still runs) |
| **Alert events generated?** | **No** | **Yes**, but suppressed |
| **Alert visible in UI?** | **No** (not loaded) | **Yes**, with its current status |
| **Use case** | Permanent removal | Temporary quiet periods |

### 4.2.1 How Silencing Works

When you silence an alert:
1. The alert **continues to evaluate** normally
2. Status transitions still happen (CLEAR → WARNING → CRITICAL)
3. **No notifications are sent** to recipients
4. The alert **appears in the UI** with its actual status
5. Events are **still recorded** in the log

Silencing is like putting a "Do Not Disturb" sign on an alert—it keeps working, but nobody gets notified.

### 4.2.2 When to Use Each

| Scenario | Approach |
|----------|----------|
| Alert monitors a service you don't run | **Disable** (never evaluate) |
| Alert is replaced by a custom version | **Disable** (permanent) |
| Nighttime maintenance window (1-4 AM) | **Silence** (temporary) |
| On-call person is on vacation | **Silence** (personal override) |
| Alert fires frequently during peak hours | **Silence** + review thresholds |
| You want to test an alert without spam | **Silence** during testing |

## 4.3 Silencing in Netdata Cloud

Netdata Cloud provides **silencing rules** that let you suppress notifications space-wide without modifying configuration files on individual nodes.

### 4.3.1 Creating Silencing Rules

**Via Cloud UI:**

1. Log in to Netdata Cloud
2. Navigate to **Settings** → **Silencing Rules** (or use the global search)
3. Click **+ Create Silencing Rule**
4. Configure the rule:

| Field | Description | Example |
|-------|-------------|---------|
| **Rule Name** | Descriptive identifier | `Weekend Maintenance` |
| **Match Scope** | What to silence | Alert name, context, node, labels |
| **Schedule** | When rule is active | `Every Saturday 1:00 AM to Monday 6:00 AM` |
| **Duration** | Optional fixed duration | `4 hours` |

**Example Rule:**

```yaml
# Silence all disk alerts on production nodes during maintenance
name: Production Maintenance
scope:
  nodes: env:production
  alerts: *disk*
schedule:
  - every: Saturday 1:00 AM
    to: Monday 6:00 AM
```

### 4.3.2 Silencing Patterns

Silencing rules support pattern matching:

| Pattern | Matches | Example |
|---------|---------|---------|
| `*` | Any characters | `*cpu*` matches `high_cpu` and `cpu_usage` |
| `?` | Single character | `disk_?` matches `disk_a`, `disk_b` |
| `\|` | OR logic | `mysql\|postgres\|redis` matches any of these |

### 4.3.3 Personal Silencing Rules

Each user can create **personal silencing rules** that only affect notifications sent to them:

1. Click your **profile icon** → **Notification Settings**
2. Create a personal silencing rule
3. Scope it to alerts you want to quiet

This is useful when:
- You're on vacation or on-call rotation
- You want to reduce noise from certain alert types
- You're debugging and don't want alerts firing to your device

### 4.3.4 Verifying Silencing Is Active

**Check in Cloud UI:**

1. Navigate to **Settings** → **Silencing Rules**
2. Look for active rules (shown with a green indicator)
3. Check the **Next Activation** time for scheduled rules

**Check via API:**

```bash
# List active silencing rules
curl -s "http://localhost:19999/api/v1/alarms?silenced=1" | jq '.'
```

## 4.4 Reducing Flapping and Noise

Alert flapping—rapidly switching between CLEAR, WARNING, and CRITICAL states—creates noise and erodes trust in alerts. Users start to ignore flapping alerts, which defeats the purpose of alerting.

### 4.4.1 Understanding Flapping

**What causes flapping:**

| Cause | Example |
|-------|---------|
| **Threshold too close to normal values** | Alert fires at >80%, system hovers at 79-81% |
| **Short evaluation window** | 10-second window catches momentary spikes |
| **Noisy metrics** | Metrics with high variance (e.g., network packets) |
| **Race conditions** | Alert fires just before scheduled maintenance |

**The cost of flapping:**

- Notification fatigue → responders ignore alerts
- False negatives → real alerts get lost in noise
- Wasted time → investigating non-issues

### 4.4.2 The `delay` Line

The `delay` line controls how long conditions must **hold** before the alert changes status. This prevents brief excursions from triggering status changes.

**Syntax:**

```conf
delay: [up|down] [seconds] [max]
```

**Parameters:**

| Param | Description | Default |
|-------|-------------|---------|
| `up` | Delay before entering WARNING/CRITICAL | Required |
| `down` | Delay before returning to CLEAR | Optional |
| `seconds` | Delay duration | Required |
| `max` | Maximum total delay accumulated | Optional |

**Example: Basic Delay**

```conf
template: high_cpu
   on: system.cpu
   lookup: average -5m of user,system
   every: 1m
   warn: $this > 80
   crit: $this > 95
   delay: up 5m down 1m
```

This means:
- **Up delay (5 minutes):** CPU must exceed threshold for **5 minutes** before WARNING/CRITICAL fires
- **Down delay (1 minute):** CPU must stay below threshold for **1 minute** before returning to CLEAR

**Example: Aggressive Delay for Unstable Metrics**

```conf
template: network_errors
   on: net.net
   lookup: average -1m of errors
   every: 1m
   warn: $this > 10
   crit: $this > 100
   delay: up 30m down 5m
   repeat: 6h
```

For metrics that naturally fluctuate, use longer delays to ensure sustained issues.

### 4.4.3 The `repeat` Line

The `repeat` line controls **how often notifications are sent** while the alert remains in a non-CLEAR state. Without this, you'll receive continuous notifications for as long as the alert is active.

**Syntax:**

```conf
repeat: [warning] [critical] [all]
```

**Parameters:**

| Param | Description | Default |
|-------|-------------|---------|
| `warning` | Interval between WARNING notifications | Required |
| `critical` | Interval between CRITICAL notifications | Defaults to `warning` value |
| `all` | Apply same interval to all severities | Optional flag |

**Example: Daily Repeat for Sustained Issues**

```conf
template: disk_space_low
   on: disk.space
   lookup: average -1m percentage of avail
   every: 1m
   warn: $this < 20
   crit: $this < 10
   repeat: 24h
   info: Disk space is running low
```

This sends notifications once per day while the alert is active—not once per minute.

### 4.4.4 Combining Delay and Repeat

These two lines work together:

```conf
template: service_health
   on: health.service
   lookup: average -3m of status
   every: 1m
   crit: $this == 0
   delay: up 10m down 2m
   repeat: 6h
```

**Behavior:**
1. Service must be down for **10 minutes** before CRITICAL fires
2. First notification sends immediately when CRITICAL fires
3. If service stays down, next notification in **6 hours**
4. Once service recovers, must stay up for **2 minutes** before CLEAR

### 4.4.5 Status-Dependent Conditions

For advanced hysteresis, use `if` conditions that depend on the **current status**:

```conf
template: cpu_trend
   on: system.cpu
   lookup: average -5m of user,system
   every: 1m
   warn: $this > ($status != CRITICAL ? 80 : 90)
   crit: $this > ($status != CLEAR ? 95 : 98)
```

This means:
- **Entering WARNING:** needs >80% CPU
- **Staying in WARNING:** tolerates up to 90% before CRITICAL
- **Entering CRITICAL:** needs >95%
- **Staying in CRITICAL:** tolerates up to 98%

See **8.1 Hysteresis and Status-Based Conditions** for more examples.

## Key Takeaway

Controlling alert noise is about **the right tool for the right job**:

- **Disable** alerts that are never relevant
- **Silence** alerts temporarily during known maintenance or quiet periods
- **Delay** before entering non-CLEAR states to prevent flapping
- **Repeat** notifications sparingly for sustained conditions

## What's Next

- **Chapter 5: Receiving Notifications** How to route alerts to people and systems
- **Chapter 6: Alert Examples and Common Patterns** Practical alert templates for common use cases
- **Chapter 7: Troubleshooting Alert Behaviour** Debugging alerts that don't behave as expected
- **Chapter 8: Advanced Alert Techniques** Hysteresis, multi-dimensional alerts, and custom actions