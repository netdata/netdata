# 7.1 Alert Never Triggers

The alert stays `CLEAR` even when you expect it to fire.

## 7.1.1 Checklist

1. Verify the alert is loaded:
```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alarms | keys[]' | grep "your_alert"
```

2. Check current value:
```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alerts.your_alert_name'
```

3. Verify the chart exists:
```bash
curl -s "http://localhost:19999/api/v1/charts" | jq '.charts[] | select(.context == "your_context")'
```

## 7.1.2 Common Causes

| Cause | Fix |
|-------|-----|
| Wrong chart/context | Find exact chart name via API |
| Missing dimension | List dimensions: `curl /api/v1/charts` |
| Threshold never met | Verify values are achievable |
| Health disabled | Check `netdata.conf` → `[health] enabled = yes` |

## 7.1.3 Related Sections

- **7.2 Always Critical** for threshold issues
- **7.4 Variables Not Found** for chart lookup problems