# 5.3 Configuring Cloud Notifications

Cloud-dispatched notifications are managed through the Netdata Cloud UI rather than configuration files.

## 5.3.1 Accessing Notification Configuration

1. Log in to Netdata Cloud
2. Navigate to **Settings** → **Notification Integrations**
3. Click **+ Add Integration**

## 5.3.2 Supported Cloud Integrations

| Integration | Type | Use Case |
|-------------|------|----------|
| **Slack** | Channel | Team chat workflows |
| **Microsoft Teams** | Channel | Enterprise collaboration |
| **Email** | Address | Traditional notification delivery |
| **PagerDuty** | Service | On-call rotation management |
| **OpsGenie** | Service | Alert routing to on-call engineers |
| **Webhook** | URL | Custom integrations, SIEM systems |
| **Jira** | Project | Incident tracking and ticket creation |

## 5.3.3 Setting Up Slack in Cloud

1. Navigate to **Settings** → **Notification Integrations**
2. Click **+ Add Slack**
3. Enter the webhook URL
4. Select the default channel
5. Click **Save**

## 5.3.4 Cloud Notification Tiers

Netdata Cloud supports multiple notification tiers:

| Tier | Description | Example |
|------|-------------|---------|
| **Personal** | Notifications sent only to you | Your phone, your email |
| **Space** | Notifications to the space's configured integrations | `#alerts` channel for the Space |
| **Role-Based** | Notifications to users with specific roles | `on-call`, `sre-team` |

## 5.3.5 Related Sections

- **[5.1 Notification Concepts](1-notification-concepts.md)** - Dispatch model overview
- **[5.4 Controlling Recipients](4-controlling-recipients.md)** - Severity routing
- **[10.2 Silencing Rules Manager](../cloud-alert-features/3-silencing-rules.md)** - Cloud silencing features