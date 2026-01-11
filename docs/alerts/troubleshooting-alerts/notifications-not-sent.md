# 7.5 Notifications Not Being Sent

The alert fires but no one receives notifications.

## 7.5.1 Is It Evaluation or Notification?

```bash
curl -s "http://localhost:19999/api/v1/alarms" | jq '.alerts.your_alert_name'
```

- Status changes? → Evaluation works, notification problem
- Status never changes? → Evaluation problem

## 7.5.2 Notification Checklist

1. Is recipient defined? `to: sysadmin` not `to: silent`
2. Is channel enabled? `SEND_SLACK=YES`
3. Check logs: `tail /var/log/netdata/error.log | grep notification`

## 7.5.3 Common Issues

| Issue | Fix |
|-------|-----|
| Silent recipient | Change `to: sysadmin` |
| Channel disabled | Enable in `health_alarm_notify.conf` |
| Wrong severity | Configure WARNING routing |

## 7.5.4 Related Sections

- **5.5 Testing and Troubleshooting** for delivery verification
- **12.2 Notification Strategy** for routing best practices