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
# Enable email notifications: YES, NO, or AUTO (auto-detect based on sendmail availability)
SEND_EMAIL="AUTO"

# Sender address for email notifications
EMAIL_SENDER="netdata@hostname"

# Default recipients (can be overridden per alert)
DEFAULT_RECIPIENT_EMAIL="admin@example.com"
```

By default, Netdata uses the system's `sendmail` command. If not found, email notifications are silently disabled. For systems without a local MTA, configure sendmail to relay to your mail server.


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
# Set the PagerDuty service key as the recipient value
DEFAULT_RECIPIENT_PD="your-pagerduty-service-key"
```

## 5.2.6 Using Custom Scripts with `exec`

The `exec:` line runs a custom script when an alert fires. This is useful for integrating with incident management systems or custom workflows:

```conf
# Example: Custom script for critical systemd service failures
template: systemd_service_unit_failed_state
      on: systemd.service_unit_state
   class: Errors
    type: Linux
component: Systemd units
chart labels: unit_name=!*
     calc: $failed
    units: state
    every: 10s
     warn: $this != nan AND $this == 1
    delay: down 5m multiplier 1.5 max 1h
  summary: systemd unit ${label:unit_name} state
     info: systemd service unit in the failed state
      exec: /usr/lib/netdata/custom/alert-handler.sh
       to: incident-response
```

See **8.4 Custom Actions with `exec`** for full details on available positional arguments and script integration patterns.

## 5.2.7 Related Sections

- **[5.1 Notification Concepts](/docs/alerts/receiving-notifications/1-notification-concepts.md)** - Understanding dispatch models
- **[5.3 Cloud Notifications](/docs/alerts/receiving-notifications/3-cloud-notifications.md)** - Cloud-based configuration
- **[8.4 Custom Actions with `exec`](/docs/alerts/essential-patterns/README.md)** - Script integration