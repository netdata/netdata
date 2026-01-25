# Expected Sidebar Structure on learn.netdata.cloud

## CURRENT STATE (based on folders in learn/)

There are TWO separate sections in learn/ folder:

### 1. Root level "Notifications" section
Located at: `learn/docs/alerts-&-notifications/notifications/`
- Agent Dispatched Notifications (28 integration files)
- Centralized Cloud Notifications (15 integration files)

### 2. "The Book" chapters (12 total)
Located at: `learn/docs/alerts-&-notifications/the-book/`
- Understanding Alerts
- Creating and Managing Alerts
- Alert Configuration Syntax
- Controlling Alerts and Noise
- **Receiving Notifications**
- Alert Examples and Common Patterns
- Troubleshooting Alert Behavior
- Essential Alert Patterns
- APIs for Alerts and Events
- Netdata Cloud Alert and Events Features
- Best Practices for Alerting
- Alerts and Notifications Architecture

---

## MAP.CSVSECTIONS TO EDIT

### Placeholder 1: Line 161
Replace `agent_notifications_integrations,,,,,` with 28 entries for Agent integrations.

**All Agent integrations (28):**
alerta, aws-sns, custom, discord, dynatrace, email, flock, gotify, ilert, irc, kavenegar, matrix, microsoft-teams, messagebird, ntfy, opsgenie, pagerduty, prowl, pushbullet, pushover, rocketchat, signal4, slack, smseagle, sms, syslog, telegram, twilio

### Placeholder 2: Line 165
Replace `cloud_notifications_integrations,,,,,` with 15 entries for Cloud integrations.

**All Cloud integrations (15):**
amazon-sns, discord, ilert, mattermost, microsoft-teams, netdata-mobile-app, opsgenie, pagerduty, rocketchat, servicenow, slack, splunk, splunk-victorops, telegram, webhook

---

## NOTHING ELSE

- Folder structure in learn/ is already correct
- "The Book" chapters are already mapped
- Only add the 28+15 entries at lines 161 and 165