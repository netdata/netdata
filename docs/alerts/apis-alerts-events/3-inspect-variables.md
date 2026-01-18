# 9.3 Inspect Alert Variables

```bash
curl -s "http://localhost:19999/api/v1/alarm_variables?chart=system.cpu" | jq '.'
```

Shows available variables for a chart.

## 9.3.1 Response Format

```json
{
  "alarm_variables": {
    "this": 45.2,
    "status": "CLEAR",
    "user": 35.5,
    "system": 9.7
  }
}
```

## 9.3.2 Related Sections

- **[7.4 Variables Not Found](../../troubleshooting-alerts/variables-not-found.md)** for troubleshooting
- **[3.5 Variables and Special Symbols](../../alert-configuration-syntax/5-variables-and-special-symbols.md)** for reference

## What's Next

- **[9.4 Health Management](4-health-management.md)** for API-based alert control