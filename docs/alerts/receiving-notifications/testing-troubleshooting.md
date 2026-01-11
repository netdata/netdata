# 5.5 Testing and Troubleshooting Notification Delivery

## 5.5.1 Testing Agent Notifications

**Send a test notification:**

```bash
# Using netdata-ui (if available)
/usr/lib/netdata/netdata-ui send-test-notification --type slack

# Or manually trigger a test
curl -s "http://localhost:19999/api/v1/alarms?test=1"
```

**Check notification logs:**

```bash
sudo tail -n 100 /var/log/netdata/error.log | grep -i notification
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
4. Are logs showing errors? Check `/var/log/netdata/error.log`
5. Is Cloud connected? Verify Agent-Cloud link status

## 5.5.5 Related Sections

- **7.5 Notifications Not Being Sent** - Debugging guide
- **9.1 Query Current Alerts** - API-based verification
- **12.2 Notification Strategy** - Best practices