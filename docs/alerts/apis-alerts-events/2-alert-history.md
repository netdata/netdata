# 9.2 Alert History

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?after=-3600" | jq '.'
```

Returns all transitions in the last hour.

## 9.2.1 Filter by Alert

```bash
curl -s "http://localhost:19999/api/v1/alarm_log?alarm=high_cpu" | jq '.'
```

## 9.2.2 Related Sections

- **9.1 Query Current Alerts** for current status
- **10.1 Events Feed** for Cloud-based history