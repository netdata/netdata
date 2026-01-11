# 9.1 Query Current Alerts

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.'
```

Response includes alert name, status, value, thresholds.

## 9.1.1 Filter by Status

```bash
curl -s "http://localhost:19999/api/v1/alarms?status=WARNING,CRITICAL" | jq '.'
```

## 9.1.2 Related Sections

- **9.3 Inspect Variables** for variable debugging
- **7.1 Alert Never Triggers** for diagnostic approach