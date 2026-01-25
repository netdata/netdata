# Expected Sidebar Structure (ALREADY FLATTENED - No "The Book")

```
Alerts & Notifications
├── Notifications
│   ├── README (line 159)
│   ├── Agent Notifications Reference (line 160)
│   ├── Agent Dispatched Notifications
│   │   ├── alerta (ADD ENTRY)
│   │   ├── aws-sns (ADD ENTRY)
│   │   ├── custom (ADD ENTRY)
│   │   ├── discord (ADD ENTRY)
│   │   ├── dynatrace (ADD ENTRY)
│   │   ├── email (ADD ENTRY)
│   │   ├── flock (ADD ENTRY)
│   │   ├── gotify (ADD ENTRY)
│   │   ├── ilert (ADD ENTRY)
│   │   ├── irc (ADD ENTRY)
│   │   ├── kavenegar (ADD ENTRY)
│   │   ├── matrix (ADD ENTRY)
│   │   ├── microsoft-teams (ADD ENTRY)
│   │   ├── messagebird (ADD ENTRY)
│   │   ├── ntfy (ADD ENTRY)
│   │   ├── opsgenie (ADD ENTRY)
│   │   ├── pagerduty (ADD ENTRY)
│   │   ├── prowl (ADD ENTRY)
│   │   ├── pushbullet (ADD ENTRY)
│   │   ├── pushover (ADD ENTRY)
│   │   ├── rocketchat (ADD ENTRY)
│   │   ├── signal4 (ADD ENTRY)
│   │   ├── slack (ADD ENTRY)
│   │   ├── smseagle (ADD ENTRY)
│   │   ├── sms (ADD ENTRY)
│   │   ├── syslog (ADD ENTRY)
│   │   ├── telegram (ADD ENTRY)
│   │   └── twilio (ADD ENTRY)
│   │
│   ├── Centralized Cloud Notifications Reference (line 162)
│   ├── Manage notification methods (line 163)
│   ├── Manage alert notification silencing rules (line 164)
│   ├── Centralized Cloud Notifications
│   │   ├── amazon-sns (ADD ENTRY)
│   │   ├── discord (ADD ENTRY)
│   │   ├── ilert (ADD ENTRY)
│   │   ├── mattermost (ADD ENTRY)
│   │   ├── microsoft-teams (ADD ENTRY)
│   │   ├── netdata-mobile-app (ADD ENTRY)
│   │   ├── opsgenie (ADD ENTRY)
│   │   ├── pagerduty (ADD ENTRY)
│   │   ├── rocketchat (ADD ENTRY)
│   │   ├── servicenow (ADD ENTRY)
│   │   ├── slack (ADD ENTRY)
│   │   ├── splunk (ADD ENTRY)
│   │   ├── splunk-victorops (ADD ENTRY)
│   │   ├── telegram (ADD ENTRY)
│   │   └── webhook (ADD ENTRY)
│
├── Understanding Alerts
├── Creating and Managing Alerts
├── Alert Configuration Syntax
├── Controlling Alerts and Noise
├── Receiving Notifications
├── Alert Examples and Common Patterns
├── Troubleshooting Alert Behaviour
├── Essential Alert Patterns
├── APIs for Alerts and Events
├── Netdata Cloud Alert and Events Features
├── Best Practices for Alerting
└── Alerts and Notifications Architecture
```

---

## Lines to Edit in map.csv

| Line | Content | Action |
|------|---------|--------|
| 161 | `agent_notifications_integrations,,,,,` | REPLACE with 28 entries |
| 165 | `cloud_notifications_integrations,,,,,` | REPLACE with 15 entries |

---

## Integration Names (Copy-Paste)

**Agent (28):**
```
alerta, aws-sns, custom, discord, dynatrace, email, flock, gotify, ilert, irc, kavenegar, matrix, microsoft-teams, messagebird, ntfy, opsgenie, pagerduty, prowl, pushbullet, pushover, rocketchat, signal4, slack, smseagle, sms, syslog, telegram, twilio
```

**Cloud (15):**
```
amazon-sns, discord, ilert, mattermost, microsoft-teams, netdata-mobile-app, opsgenie, pagerduty, rocketchat, servicenow, slack, splunk, splunk-victorops, telegram, webhook
```

---

## That's It

- Folder structure in learn/ already correct
- 12 chapters already mapped
- JUST add entries at lines 161 and 165