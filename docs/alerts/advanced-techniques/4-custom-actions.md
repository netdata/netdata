# 8.4 Custom Actions with `exec`

## 8.4.1 Basic exec Syntax

```conf
template: service_down
    on: systemd.service_unit_state
lookup: average -1m of value
    every: 1m
     crit: $this == 0
       exec: /usr/local/bin/alert-handler.sh
       to: ops-team
```

## 8.4.2 Argument Position

The exec script receives 34 positional arguments (not environment variables):

| Position | Description |
|----------|-------------|
| 1 | exec script path |
| 2 | recipient |
| 3 | registry hostname |
| 4 | unique_id |
| 5 | alarm_id |
| 6 | alarm_event_id |
| 7 | when (Unix timestamp) |
| 8 | alert_name |
| 9 | alert_chart_name |
| 10 | new_status (CLEAR, WARNING, CRITICAL) |
| 11 | old_status |
| 12 | new_value |
| 13 | old_value |
| 14 | alert_source |
| 15 | duration |
| 16 | non_clear_duration |
| 17 | alert_units |
| 18 | alert_info |
| 19 | new_value_string |
| 20 | old_value_string |
| 21 | source |
| 22 | error_msg |
| 23 | n_warn |
| 24 | n_crit |
| 25 | warn_alarms |
| 26 | crit_alarms |
| 27 | classification |
| 28 | edit_command |
| 29 | machine_guid |
| 30 | transition_id |
| 31 | summary |
| 32 | context |
| 33 | component |
| 34 | type |

## 8.4.3 Example: PagerDuty Integration

```bash
#!/bin/bash
# Position 10 = new_status
if [ "${10}" = "CRITICAL" ]; then
    curl -X POST https://events.pagerduty.com/v2/enqueue \
        -H 'Content-Type: application/json' \
        -d "{\"routing_key\": \"YOUR_KEY\", \"event_action\": \"trigger\", \"dedup_key\": \"${8}\"}"
fi
```

## 8.4.4 Related Sections

- **5.2.6 Custom Scripts** for additional examples