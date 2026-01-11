# 8.4 Custom Actions with `exec`

## 8.4.1 Basic exec Syntax

```conf
template: service_down
    on: health.service
lookup: average -1m of status
    every: 1m
     crit: $this == 0
       exec: /usr/local/bin/alert-handler.sh
       to: ops-team
```

## 8.4.2 Environment Variables

| Variable | Description |
|----------|-------------|
| `NETDATA_ALARM_NAME` | Alert name |
| `NETDATA_HOST` | Hostname |
| `NETDATA_STATUS` | Current status |
| `NETDATA_VALUE` | Current value |

## 8.4.3 Example: PagerDuty Integration

```bash
#!/bin/bash
if [ "$NETDATA_STATUS" = "CRITICAL" ]; then
    curl -X POST https://events.pagerduty.com/v2/enqueue \
        -H 'Content-Type: application/json' \
        -d "{\"routing_key\": \"YOUR_KEY\", \"event_action\": \"trigger\", \"dedup_key\": \"$NETDATA_ALARM_NAME\", \"payload\": {\"summary\": \"Netdata alert $NETDATA_ALARM_NAME\", \"source\": \"$NETDATA_HOST\", \"severity\": \"critical\"}}"
fi
```

## 8.4.4 Related Sections

- **5.2.6 Custom Scripts** for additional examples