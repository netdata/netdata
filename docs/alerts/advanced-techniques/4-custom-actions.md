# 8.4 Custom Actions with `exec`

## 8.4.1 Basic exec Syntax

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

## 8.4.2 Argument Position

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

## 8.4.3 Example: PagerDuty Integration

```bash
#!/bin/bash
# Position 10 = new_status
if [ "${10}" = "CRITICAL" ]; then
    curl -X POST https://events.pagerduty.com/v2/enqueue \
        -H 'Content-Type: application/json' \
        -d "{\"routing_key\": \"YOUR_KEY\", \"event_action\": \"trigger\", \"dedup_key\": \"${3}\"}"
fi
```

## 8.4.4 Related Sections

- **[5.2.6 Custom Scripts](../receiving-notifications/2-agent-parent-notifications.md)** for additional examples

## What's Next

- **[8.5 Performance Considerations](5-performance.md)** for optimizing alert evaluation
- **[Chapter 13: Architecture](../architecture/index.md)** for deep-dive internals