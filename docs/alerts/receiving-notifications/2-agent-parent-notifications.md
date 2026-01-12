# 5.2 Configuring Agent and Parent Notifications

Notifications from Agents and Parents are configured in `health_alarm_notify.conf`.

## 5.2.1 Finding the Configuration File

```bash
sudo less /etc/netdata/health_alarm_notify.conf
```

This file ships with Netdata and contains the notification subsystem configuration.

## 5.2.2 Notification Methods

Netdata supports multiple notification destinations:

| Method | Type | Setup Complexity | Best For |
|--------|------|------------------|----------|
| **Email** | Built-in | Low | Pager schedules, management |
| **Slack** | Webhook | Low | DevOps teams, chat workflows |
| **Discord** | Webhook | Low | Gaming teams, community ops |
| **PagerDuty** | Integration | Medium | On-call rotations, enterprise |
| **OpsGenie** | Integration | Medium | Enterprise incident management |
| **Telegram** | Bot | Medium | Direct message alerts |
| **Custom Scripts** | exec | High | Any custom integration |

## 5.2.3 Configuring Email Notifications

```conf
# Enable email notifications
SEND_EMAIL="YES"

# SMTP configuration (for systems without local MTA)
SMTP_SERVER="smtp.example.com:587"
SMTP_USER="alerts@example.com"
SMTP_PASSWORD="your-password"
FROM_ADDRESS="netdata@example.com"

# Default recipients (can be overridden per alert)
DEFAULT_RECIPIENT_EMAIL="admin@example.com"
```

## 5.2.4 Configuring Slack Notifications

**Step 1: Create a Slack Incoming Webhook**

1. Go to your Slack workspace → **Settings & Admin** → **Manage apps**
2. Create a new incoming webhook
3. Select the channel for alerts
4. Copy the webhook URL

**Step 2: Configure Netdata**

```conf
SEND_SLACK="YES"
SLACK_WEBHOOK_URL="https://hooks.slack.com/services/YOUR/WEBHOOK/URL"
DEFAULT_RECIPIENT_SLACK="#alerts"
```

## 5.2.5 Configuring PagerDuty

```conf
SEND_PD="YES"
PD_SERVICE_KEY="your-service-key"
DEFAULT_RECIPIENT_PD=" pagerduty-group"
```

## 5.2.6 Using Custom Scripts with `exec`

```conf
template: custom_alert
    on: systemd.service_unit_state
    lookup: average -1m of status
    every: 1m
    crit: $this == 0
    exec: /usr/lib/netdata/custom/alert-handler.sh
    to: custom-recipient
```

See **8.4 Custom Actions with `exec`** for full details.

## 5.2.7 Related Sections

- **5.1 Notification Concepts** - Understanding dispatch models
- **5.3 Cloud Notifications** - Cloud-based configuration
- **8.4 Custom Actions with `exec`** - Advanced script integration