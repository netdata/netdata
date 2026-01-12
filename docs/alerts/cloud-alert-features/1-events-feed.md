# 10.1 Events Feed

1. Log in to Netdata Cloud
2. Navigate to **Events** in the left sidebar

## 10.1.1 Event Types

| Type | Description |
|------|-------------|
| Alert Created | New alert defined |
| Alert Triggered | Status changed to WARNING/CRITICAL |
| Alert Cleared | Returned to CLEAR |

## 10.1.2 Filtering

```text
status:CRITICAL  # Only critical alerts
host:prod-db-*   # Database servers
alert:*cpu*      # CPU-related alerts
```

## 10.1.3 Related Sections

- **9.5 Cloud Events API** for programmatic access
- **5.3 Cloud Notifications** for routing