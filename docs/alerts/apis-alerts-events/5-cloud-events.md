# 9.5 Cloud Events API

The Cloud Events API (`/api/v2/events`) queries alert and event history stored in Netdata Cloud, providing aggregated, multi-node event data that requires authentication.

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

- **[10.1 Events Feed](../cloud-alert-features/1-events-feed.md)** for Cloud UI features
- **[5.3 Cloud Notifications](../../receiving-notifications/3-cloud-notifications.md)** for Cloud routing

## What's Next

- **[Chapter 10: Cloud Alert Features](../cloud-alert-features/index.md)** for more Cloud capabilities