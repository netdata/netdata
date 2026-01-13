# 9.2 Alert History

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?after=${LAST_UNIQUEID}" | jq '.'
```

Returns all transitions in the last hour.

## 9.2.1 Filter by Chart

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?chart=system.cpu" | jq '.'
```

## 9.2.2 Related Sections

- **[9.1 Query Current Alerts](1-query-alerts.md)** for current status
- **[9.3 Inspect Variables](3-inspect-variables.md)** for variable debugging

## What's Next

- **[10.1 Events Feed](../cloud-alert-features/1-events-feed.md)** for Cloud-based history