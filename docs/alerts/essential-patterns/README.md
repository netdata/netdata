# 8. Essential Alert Patterns

This chapter covers standard techniques for effective alerting used in most production environments.

## Here

| Section | Technique |
|---------|-----------|
| **[8.1 Hysteresis and Status-Based Conditions](#81-hysteresis-and-status-based-conditions)** | Status-dependent thresholds |
| **[8.2 Multi-Dimensional and Per-Instance Alerts](#82-multi-dimensional-and-per-instance-alerts)** | Target specific dimensions or instances |
| **[8.3 Host, Chart, and Label-Based Targeting](#83-host-chart-and-label-based-targeting)** | Fine-grained scoping with labels |
| **[8.4 Custom Actions with `exec`](#84-custom-actions-with-exec)** (Advanced) | `exec` for automation scripts |
| **[8.5 Performance Considerations](#85-performance-considerations)** (Advanced) | Optimizing alert evaluation |

## 8.1 Hysteresis and Status-Based Conditions

Hysteresis prevents alert flapping by using different thresholds for entering and clearing states based on current alert status.

### 8.1.1 The Problem with Simple Thresholds

```conf
warn: $this > 80
```

This causes flapping when CPU hovers around 80%.

### 8.1.2 Hysteresis Solution

```conf
template: 10min_cpu_usage
    on: system.cpu
lookup: average -10m unaligned of user,system
     units: %
     every: 1m
      warn: ($status == $CLEAR && $this > 80) || ($status >= $WARNING && $this > 70)
      crit: ($status < $CRITICAL && $this > 95) || ($status == $CRITICAL && $this > 90)
        ok: $this < 70
```

This implements asymmetric hysteresis:
- Enter WARNING at 80%, but stay WARNING until > 85%
- Enter CRITICAL at 95%, but stay CRITICAL until > 98%
- Clear only when below 70%

The `$status` variable enables different thresholds based on current state.

## 8.2 Multi-Dimensional and Per-Instance Alerts

Multi-dimensional alerts target specific dimensions within a chart, while templates create alerts for every matching chart instance automatically.

### 8.2.1 Dimension Selection

Target specific dimensions within a chart:

```conf
template: interface_inbound_errors
    on: net.errors
lookup: sum -10m unaligned absolute of inbound
     units: errors
     every: 1m
      warn: $this >= 5
```

This monitors only the `inbound` dimension of net.errors charts.

### 8.2.2 Scaling to All Instances

Templates automatically create one alert per matching chart.

```conf
template: interface_inbound_errors
    on: net.errors
lookup: average -5m of inbound
     every: 1m
      warn: $this > 10
```

Creates alerts for: `interface_inbound_errors-eth0`, `interface_inbound_errors-eth1`, etc.

## 8.3 Host, Chart, and Label-Based Targeting

Labels enable fine-grained alert scoping to specific hosts, services, or chart instances using metadata attached to nodes and charts.

### 8.3.1 Understanding Labels

Netdata supports:
- **Host labels**: `env:production`, `role=database`
- **Chart labels**: `mount_point:/data`

### 8.3.2 Label-Based Alert Scope

Labels enable scoping alerts to specific hosts or chart instances. Use `host labels:` to target hosts with specific labels:

```conf
template: 10min_cpu_usage
      on: system.cpu
   class: Utilization
    type: System
host labels: role=database
      lookup: average -10m unaligned of user,system
     units: %
     every: 1m
      warn: $this > 80
      crit: $this > 95
       to: dba-team
```

This creates alerts only for hosts with `role=database` label. The alert still fires for all matching database hosts.

## 8.4 Custom Actions with `exec`

The `exec` line triggers external scripts when alerts fire, enabling automation like paging systems or incident response workflows.

### 8.4.1 Basic exec Syntax

```conf
template: systemd_service_unit_failed_state
      on: systemd.service_unit_state
   class: Errors
    type: Linux
component: Systemd units
chart labels: unit_name=!*
      calc: $failed
     units: state
     every: 10s
      warn: $this != nan AND $this == 1
     delay: down 5m multiplier 1.5 max 1h
   summary: systemd unit ${label:unit_name} state
      info: systemd service unit in the failed state
        exec: /usr/local/bin/alert-handler.sh
         to: sysadmin
```

### 8.4.2 Argument Position

The exec script receives 33 positional arguments (not environment variables):

| Position | Description |
|----------|-------------|
| 1 | recipient |
| 2 | registry hostname |
| 3 | unique_id |
| 4 | alarm_id |
| 5 | alarm_event_id |
| 6 | when (Unix timestamp) |
| 7 | alert_name |
| 8 | alert_chart_name |
| 9 | current_status |
| 10 | new_status (CLEAR, WARNING, CRITICAL) |
| 11 | new_value |
| 12 | old_value |
| 13 | alert_source |
| 14 | duration |
| 15 | non_clear_duration |
| 16 | alert_units |
| 17 | alert_info |
| 18 | new_value_string |
| 19 | old_value_string |
| 20 | calc_expression |
| 21 | calc_param_values |
| 22 | n_warn |
| 23 | n_crit |
| 24 | warn_alarms |
| 25 | crit_alarms |
| 26 | classification |
| 27 | edit_command |
| 28 | machine_guid |
| 29 | transition_id |
| 30 | summary |
| 31 | context |
| 32 | component |
| 33 | type |

### 8.4.3 Example: PagerDuty Integration

```bash
#!/bin/bash
# Position 10 = new_status
if [ "${10}" = "CRITICAL" ]; then
    curl -X POST https://events.pagerduty.com/v2/enqueue \
        -H 'Content-Type: application/json' \
        -d "{\"routing_key\": \"YOUR_KEY\", \"event_action\": \"trigger\", \"dedup_key\": \"${3}\"}"
fi
```

## 8.5 Performance Considerations

Balance alert responsiveness with resource usage by tuning evaluation frequency, lookup windows, and alert counts.

### 8.5.1 What Affects Performance

| Factor | Impact | Recommendation |
|--------|--------|----------------|
| `every` frequency | Higher = more CPU | Use 1m for most alerts |
| `lookup` window | Longer windows need more processing | Match to needs |
| Number of alerts | More alerts = more evaluation | Disable unused alerts |

### 8.5.2 Efficient Configuration

```conf
# Efficient: 1-minute evaluation
template: 10min_cpu_usage
    on: system.cpu
lookup: average -10m unaligned of user,system
     units: %
     every: 1m     # Good: 60 evaluations/hour
      warn: $this > 80
      crit: $this > 95
```

```conf
# Inefficient: 10-second evaluation
template: 10min_cpu_usage
    on: system.cpu
lookup: average -1m unaligned of user,system
     units: %
     every: 10s    # Bad: 360 evaluations/hour
      warn: $this > 80
      crit: $this > 95
```

## Related Sections

- **[4.4 Reducing Flapping](../controlling-alerts-noise/4-reducing-flapping.md)** - Delay and repeat techniques
- **[5.2.6 Custom Scripts](../receiving-notifications/2-agent-parent-notifications.md)** - Exec and webhook examples
- **[7.3 Alert Flapping](../troubleshooting-alerts/README.md)** - Debugging alert flapping issues
- **[10.4 Room-Based Alerting](../cloud-alert-features/README.md)** - Using labels in Cloud
- **[13.1 Evaluation Architecture](../architecture/1-evaluation-architecture.md)** - Alert evaluation internals