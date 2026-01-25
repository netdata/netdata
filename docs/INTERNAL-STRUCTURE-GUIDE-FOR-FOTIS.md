# Internal Guide: Alerts & Notifications Structure (DO NOT PUBLISH)

**For:** Fotis
**Purpose:** Populate `docs/.map/map.csv` with Agent and Cloud dispatch integration entries

---

## THE MAP.CSV STRUCTURE

```
Alerts & Notifications (top-level)
├── Notifications (section header)
│   ├── README → Notifications overview (line 159)
│   ├── Agent Notifications Reference (line 160)
│   ├── agent_notifications_integrations ← PLACEHOLDER LINE 161 ← ADD HERE
│   ├── Centralized Cloud Notifications Reference (line 162)
│   ├── Manage notification methods (line 163)
│   ├── Manage alert notification silencing rules (line 164)
│   └── cloud_notifications_integrations ← PLACEHOLDER LINE 165 ← ADD HERE
├── Understanding Alerts (Chapter 1)
│   ├── README (line 169)
│   ├── What is a Netdata Alert (line 170)
│   ├── Alert Types: alarm vs template (line 171)
│   └── Where Alerts Live (line 172)
├── Creating and Managing Alerts (Chapter 2) (lines 174-179)
├── Alert Configuration Syntax (Chapter 3) (lines 181-187)
├── Controlling Alerts and Noise (Chapter 4) (lines 189-193)
├── Receiving Notifications (Chapter 5) (lines 195-200)
├── Alert Examples and Common Patterns (Chapter 6) (line 202)
├── Troubleshooting Alert Behavior (Chapter 7) (line 204)
├── Essential Alert Patterns (Chapter 8) (line 206)
├── APIs for Alerts and Events (Chapter 9) (line 208)
├── Netdata Cloud Alert and Events Features (Chapter 10) (line 210)
├── Stock Alerts Reference (Chapter 11) (line 212)
├── Best Practices for Alerting (Chapter 12) (lines 214-219)
└── Alerts and Notifications Architecture (Chapter 13) (lines 221-226)
```

---

## LINES TO POPULATE

| Line | Column B Value | Where to Insert New Entries |
|------|----------------|----------------------------|
| **161** | `agent_notifications_integrations` | Insert Agent integration entries BELOW this line |
| **165** | `cloud_notifications_integrations` | Insert Cloud integration entries BELOW this line |

---

## INTEGRATIONS TO ADD

### 1. AGENT DISPATCHED (insert below line 161)

Each entry format:
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/{integration}.mdx,{Integration Name},Published,Alerts & Notifications/Notifications/Agent Dispatched Notifications,
```

**Files to add entries for:**
| Integration | Filename |
|------------|----------|
| Alerta | alerta.mdx |
| AWS SNS | aws-sns.mdx |
| Custom | custom.mdx |
| Discord | discord.mdx |
| Dynatrace | dynatrace.mdx |
| Email | email.mdx |
| Flock | flock.mdx |
| Gotify | gotify.mdx |
| Ilert | ilert.mdx |
| IRC | irc.mdx |
| Kavenegar | kavenegar.mdx |
| Matrix | matrix.mdx |
| Microsoft Teams | microsoft-teams.mdx |
| MessageBird | messagebird.mdx |
| NTFY | ntfy.mdx |
| OpsGenie | opsgenie.mdx |
| PagerDuty | pagerduty.mdx |
| Prowl | prowl.mdx |
| Pushbullet | pushbullet.mdx |
| Pushover | pushover.mdx |
| Rocket.Chat | rocketchat.mdx |
| Signal4 | signal4.mdx |
| Slack | slack.mdx |
| SMSEagle | smseagle.mdx |
| SMS | sms.mdx |
| Syslog | syslog.mdx |
| Telegram | telegram.mdx |
| Twilio | twilio.mdx |

### 2. CLOUD DISPATCHED (insert below line 165)

Each entry format:
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/{integration}.mdx,{Integration Name},Published,Alerts & Notifications/Notifications/Centralized Cloud Notifications,
```

**Files to add entries for:**
| Integration | Filename |
|------------|----------|
| Amazon SNS | amazon-sns.mdx |
| Discord | discord.mdx |
| Ilert | ilert.mdx |
| Mattermost | mattermost.mdx |
| Microsoft Teams | microsoft-teams.mdx |
| Netdata Mobile App | netdata-mobile-app.mdx |
| OpsGenie | opsgenie.mdx |
| PagerDuty | pagerduty.mdx |
| Rocket.Chat | rocketchat.mdx |
| ServiceNow | servicenow.mdx |
| Slack | slack.mdx |
| Splunk | splunk.mdx |
| Splunk VictorOps | splunk-victorops.mdx |
| Telegram | telegram.mdx |
| Webhook | webhook.mdx |

---

## EXAMPLE ENTRIES

### Agent Email
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/email.mdx,Email,Published,Alerts & Notifications/Notifications/Agent Dispatched Notifications,
```

### Cloud Slack
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/slack.mdx,Slack,Published,Alerts & Notifications/Notifications/Centralized Cloud Notifications,
```

---

## WHAT NOT TO TOUCH

- **Old docs at:** `docs/alerts-and-notifications/` - ignore, not part of new structure
- **Source code at:** `src/health/*.c` - terminology handled elsewhere
- **Chapters 1-13** in map.csv - already populated, do not modify

---

## VERIFICATION CHECKLIST

After adding entries:
- [ ] All 28 Agent integrations have map.csv entries below line 161
- [ ] All 15 Cloud integrations have map.csv entries below line 165
- [ ] No duplicates (each filename appears exactly once)
- [ ] Spacing preserved (blank line before/after section groups)
- [ ] File paths correctly reference `learn/docs/.../...mdx`