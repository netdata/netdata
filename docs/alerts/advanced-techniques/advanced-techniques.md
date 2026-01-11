# 8. Advanced Alert Techniques

This chapter covers **advanced patterns** for experienced users who need more sophisticated alert behavior. The techniques here assume familiarity with basic alert syntax (Chapters 2-3) and notification configuration (Chapter 5).

:::warning
These techniques can create complex configurations. Use only when simpler approaches don't solve your problem.
:::

## What This Chapter Covers

| Section | Technique | Use Case |
|---------|-----------|----------|
| **8.1 Hysteresis** | Status-dependent thresholds | Enter/exit at different levels |
| **8.2 Multi-Dimensional** | Target specific dimensions | Per-disk, per-interface monitoring |
| **8.3 Label-Based Targeting** | Host and chart labels | Cluster-wide rules with label filters |
| **8.4 Custom Actions** | `exec` for automation | Run scripts on alert transitions |
| **8.5 Performance** | Efficient alert sets | Scale to thousands of alerts |

## 8.1 Hysteresis and Status-Based Conditions

Hysteresis means **entering** and **exiting** an alert state at different thresholds. This prevents flapping around a single threshold.

### 8.1.1 The Problem with Simple Thresholds

```conf
# Without hysteresis - alert fires on brief crossings
template: simple_cpu_alert
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: $this > 80
     crit: $this > 95
```

This creates flapping around 80% when CPU hovers near the threshold.

### 8.1.2 Hysteresis Solution

```conf
# Enter WARNING at 80%, but only clear when below 70%
template: cpu_hysteresis
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: ($this > 80) && ($status != WARNING)
     crit: ($this > 95) && ($status != CRITICAL)
```

**Behavior:**
- **Enter WARNING:** CPU > 80% AND wasn't already in WARNING
- **Clear WARNING:** CPU < 70%
- **Enter CRITICAL:** CPU > 95% AND wasn't already in CRITICAL
- **Clear CRITICAL:** CPU < 85%

### 8.1.3 More Complex Hysteresis

For environments with different enter/exit levels:

```conf
# Enter WARNING at 70%, stay until 60%
# Enter CRITICAL at 90%, stay until 80%
template: multi_level_hysteresis
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: ($this > 70) && ($status == CLEAR || $status == CRITICAL)
     crit: ($this > 90) && ($status == CLEAR || $status == WARNING)
```

## 8.2 Multi-Dimensional and Per-Instance Alerts

Template alerts automatically create instances for all matching charts. Use dimension filtering to target specific instances.

### 8.2.1 Dimension Selection

```conf
# Alert only on specific dimensions
template: network_quality
    on: net.net
lookup: average -1m of errors,dropped
    every: 1m
     warn: $errors > 10
     crit: $errors > 100
```

### 8.2.2 Scaling to All Instances

Templates automatically instantiate for all matching charts:

```conf
# Creates one alert per network interface
template: interface_errors
    on: net.net
lookup: average -5m of errors
    every: 1m
     warn: $this > 10
     crit: $this > 100
```

This creates alerts for:
- `interface_errors-eth0`
- `interface_errors-eth1`
- `interface_errors-eth2`
- ...all network interfaces on the host

### 8.2.3 Excluding Specific Instances

Use `if` conditions or `calc` to exclude specific charts:

```conf
# Alert on all disks except loop devices
template: disk_health
    on: disk.space
lookup: average -5m percentage of avail
    every: 1m
     warn: $this < 20
       to: ops-team
     info: Disk ${chart} has ${value}% free space
   calc: if($chart ~ "loop.*" then 100 else $this)  # Exclude loop devices
```

## 8.3 Host, Chart, and Label-Based Targeting

Use host labels to scope alerts across your infrastructure.

### 8.3.1 Understanding Labels

Netdata supports:
- **Host labels**: Tags applied to entire nodes (e.g., `env:production`, `role:database`)
- **Chart labels**: Tags applied to specific charts (e.g., `mount_point:/data`)

### 8.3.2 Label-Based Alert Scope

```conf
# Only alert on production database servers
template: db_cpu_high
    on: system.cpu
lookup: average -5m of user,system
    every: 1m
     warn: $this > 80
     crit: $this > 95
   calc: if($host_labels.role == "database" && $host_labels.env == "production", $this, 0)
```

### 8.3.3 Dynamic Scope with Labels

Labels are evaluated at alert creation time. Use Cloud UI for dynamic label-based targeting that applies to new nodes automatically.

## 8.4 Custom Actions with `exec`

The `exec` line runs a script or command when an alert changes status.

### 8.4.1 Basic exec Syntax

```conf
template: service_down
    on: health.service
lookup: average -1m of status
    every: 1m
     crit: $this == 0
       exec: /usr/local/bin/alert-handler.sh
       to: ops-team
```

### 8.4.2 Environment Variables

The script receives these environment variables:

| Variable | Description |
|----------|-------------|
| `NETDATA_ALARM_NAME` | Alert/template name |
| `NETDATA_HOST` | Hostname |
| `NETDATA_CHART` | Chart ID |
| `NETDATA_CONTEXT` | Chart context |
| `NETDATA_STATUS` | New status (CLEAR, WARNING, CRITICAL) |
| `NETDATA_PREVIOUS_STATUS` | Previous status |
| `NETDATA_VALUE` | Current value |
| `NETDATA_REASON` | Human-readable reason |
| `NETDATA_TIME` | Unix timestamp |
| `NETDATA_UNIXTIME` | Formatted time |

### 8.4.3 Example: PagerDuty Integration

```bash
#!/bin/bash
# /usr/local/bin/pagerduty-trigger.sh

if [ "$NETDATA_STATUS" = "CRITICAL" ]; then
    curl -X POST https://events.pagerduty.com/v2/enqueue \
        -H 'Content-Type: application/json' \
        -d '{
            "routing_key": "YOUR_PD_KEY",
            "event_action": "trigger",
            "dedup_key": "netdata-${NETDATA_HOST}-${NETDATA_ALARM_NAME}",
            "payload": {
                "summary": "${NETDATA_ALARM_NAME} on ${NETDATA_HOST}: ${NETDATA_STATUS}",
                "source": "${NETDATA_HOST}",
                "severity": "critical"
            }
        }'
fi
```

### 8.4.4 Exit Code Handling

```bash
#!/bin/bash
# Return code determines notification behavior
# 0 = success (notification sent)
# 1 = non-critical failure
# 2+ = critical failure (alert marked as failed)
```

Return code 2 or higher marks the exec as failed and logs to `/var/log/netdata/error.log`.

### 8.4.5 Webhook Endpoints with `exec`

```conf
# Send webhook on alert
template: security_alert
    on: security.event
lookup: average -1m of count
    every: 1m
     crit: $this > 100
       exec: /bin/curl -X POST https://your-webhook.example.com/alerts \
             -H 'Content-Type: application/json' \
             -d "{\"alert\": \"$NETDATA_ALARM_NAME\", \"host\": \"$NETDATA_HOST\", \"status\": \"$NETDATA_STATUS\"}"
```

## 8.5 Performance Considerations for Large Alert Sets

When managing hundreds or thousands of alerts, performance matters.

### 8.5.1 What Affects Performance

| Factor | Impact | Recommendation |
|--------|--------|-----------------|
| `every` frequency | Higher frequency = more CPU | Use 1m for most alerts, 10s only for critical metrics |
| `lookup` window | Longer windows require more data processing | Match to your alerting needs |
| `calc` complexity | Complex expressions add CPU | Keep expressions simple |
| Number of alerts | More alerts = more evaluation | Disable unused alerts |

### 8.5.2 Performance Tuning

```conf
# Efficient: 1-minute evaluation, 5-minute window
template: system_health
    on: system.cpu
lookup: average -5m of user,system
    every: 1m    # Good: 60 evaluations per hour
     warn: $this > 80
     crit: $this > 95

# Inefficient: 10-second evaluation, 1-minute window
template: system_health_fast
    on: system.cpu
lookup: average -1m of user,system
    every: 10s   # Bad: 360 evaluations per hour, same data
     warn: $this > 80
```

### 8.5.3 Disabling Unused Alerts

For large deployments, disable entire categories:

```conf
# Disable alerts you don't need
alarm: some_unused_alarm
   enabled: no
```

Or use silencing rules (Chapter 4) to suppress notifications without disabling evaluation.

### 8.5.4 Alert Grouping

Group related alerts in the same file for easier management:

```conf
# /etc/netdata/health.d/database-alerts.conf

template: mysql_slow_queries
    on: mysql.global_status
lookup: average -5m of slow_queries
    every: 1m
     warn: $this > 10

template: mysql_connections
    on: mysql.connections
lookup: average -5m of connected
    every: 1m
     warn: $this > 1000
```

## Key Takeaway

Advanced techniques give you precise control over alert behavior. Use hysteresis for stable thresholds, exec for automation, and consider performance when scaling to many alerts.

## What's Next

- **Chapter 9: APIs for Alerts and Events** Programmatic control over alerts
- **Chapter 10: Netdata Cloud Alert Features** Cloud-specific advanced features
- **Chapter 13: Alerts and Notifications Architecture** Deep-dive into internal behavior