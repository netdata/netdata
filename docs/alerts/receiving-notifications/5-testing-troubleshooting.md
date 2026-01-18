# 5.5 Testing and Troubleshooting Notification Delivery

## 5.5.1 Testing Agent Notifications

**Send a test notification:**

```bash
# Test all configured notification methods
sudo bash /usr/lib/netdata/health/alarm-test.sh

# Test a specific notification type (e.g., slack, email, pagerduty)
sudo bash /usr/lib/netdata/health/alarm-test.sh slack
```

**Check notification logs:**

```bash
sudo tail -n 100 /var/log/netdata/health.log | grep -i notification
```

## 5.5.2 Testing Cloud Notifications

**Create a test alert in Cloud UI:**

1. Navigate to **Alerts** → **Alert Configuration**
2. Create a test alert with low thresholds
3. Monitor the target integration

## 5.5.3 Common Issues

| Issue | Likely Cause | Fix |
|-------|--------------|-----|
| No notifications received | Recipient misconfigured | Verify `to:` field |
| Duplicate notifications | Both Agent and Cloud sending | Disable one dispatch model |
| Notifications delayed | Network latency or batching | Check connection status |
| Wrong channel | Routing misconfigured | Verify channel name in config |
| Alerts not triggering | Thresholds too high | Lower threshold or check metrics |

## 5.5.4 Debugging Checklist

1. Is the alert firing? Check API: `curl http://localhost:19999/api/v1/alarms`
2. Is the recipient correct? Check `to:` line
3. Is the notification method enabled? Verify `SEND_SLACK=YES`
4. Are logs showing errors? Check `/var/log/netdata/health.log`
5. Is Cloud connected? Verify Agent-Cloud link status

## 5.5.5 Related Sections

- **[7.5 Notifications Not Being Sent](../../troubleshooting-alerts/notifications-not-sent.md)** - Debugging guide
- **[9.1 Query Current Alerts](../../apis-alerts-events/1-query-alerts.md)** - API-based verification
- **[12.2 Notification Strategy and On-Call Hygiene](../best-practices/2-notification-strategy.md)** - Best practices