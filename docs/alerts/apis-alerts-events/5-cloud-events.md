# 9.5 Cloud Events API

```bash
curl -s "https://app.netdata.cloud/api/v2/events?space_id=YOUR_SPACE" \
  -H "Authorization: Bearer YOUR_TOKEN" | jq '.'
```

## 9.5.1 Parameters

| Parameter | Description |
|-----------|-------------|
| `space_id` | Filter by Space |
| `alert_name` | Filter by alert pattern |
| `status` | Filter by severity |

## 9.5.2 Related Sections

- **10.1 Events Feed** for Cloud UI features
- **5.3 Cloud Notifications** for Cloud routing