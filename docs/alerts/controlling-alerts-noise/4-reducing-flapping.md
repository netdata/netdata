# 4.4 Reducing Flapping and Noise

Alert flapping—rapidly switching between CLEAR, WARNING, and CRITICAL states—creates noise and erodes trust in alerts. Users start to ignore flapping alerts, which defeats the purpose of alerting.

## 4.4.1 Understanding Flapping

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

## 4.4.2 The `delay` Line

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

## 4.4.3 The `repeat` Line

The `repeat` line controls **how often notifications are sent** while the alert remains in a non-CLEAR state.

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

## 4.4.4 Combining Delay and Repeat

```conf
template: port_response_slow
    on: portcheck.status
    lookup: average -3m of time
    every: 1m
    warn: $this > 1000
    delay: up 10m down 2m
    repeat: 6h
```

**Behavior:**
1. Service must be down for **10 minutes** before CRITICAL fires
2. First notification sends immediately when CRITICAL fires
3. If service stays down, next notification in **6 hours**
4. Once service recovers, must stay up for **2 minutes** before CLEAR

## 4.4.5 Status-Dependent Conditions

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

See **8.1 Hysteresis and Status-Based Conditions** for more examples of status-dependent thresholds.

## 4.4.6 Related Sections

- **8.1 Hysteresis and Status-Based Conditions** - Advanced status-aware logic
- **7.3 Alert Flapping** - Debugging and fixing flapping alerts
- **5.5 Testing and Troubleshooting** - Verifying notification delivery