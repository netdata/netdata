# 9.1 Query Current Alerts

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

Response includes alert name, status, value, thresholds.

## 9.1.1 Filter Alerts

```bash
# Get all alerts including cleared
curl -s "http://localhost:19999/api/v1/alarms?all" | jq '.'

# Get only active alerts (default)
curl -s "http://localhost:19999/api/v1/alarms?active" | jq '.'
```

## 9.1.2 Related Sections

- **[9.2 Alert History](2-alert-history.md)** for transition history
- **[9.3 Inspect Variables](3-inspect-variables.md)** for variable debugging

## What's Next

- **[7.1 Alert Never Triggers](../troubleshooting-alerts/alert-never-triggers.md)** for diagnostic approach