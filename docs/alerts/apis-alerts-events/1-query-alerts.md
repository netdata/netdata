# 9.1 Query Current Alerts

The `/api/v1/alarms` endpoint returns all currently active alerts on the agent. Response includes each alert's name, status, current value, and configured threshold.

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

Response includes alert name, status, value, thresholds.

## 9.1.1 Filter Alerts

```bash
# Get all alerts including cleared
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.'

# Get only active alerts (default)
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

## What's Next

- **[9.2 Alert History](2-alert-history.md)** for alert transitions
- **[9.3 Inspect Variables](3-inspect-variables.md)** for variable debugging
- **[10.1 Events Feed](../cloud-alert-features/1-events-feed.md)** for Cloud-based history
