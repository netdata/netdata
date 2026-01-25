# Internal Guide for Fotis: Add Integration Entries to map.csv

**Your task:** Add Agent and Cloud integration entries to `docs/.map/map.csv`

---

## WHERE TO ADD ENTRIES

There are TWO places in map.csv that need entries:

### 1. Agent-Dispatched Integrations
Insert entries **between line 160 and line 162**

Current content around that area:
- Line 160: Agent Notifications Reference (already exists)
- **Line 161: PLACEHOLDER** `agent_notifications_integrations,,,,,`
- Line 162: Centralized Cloud Notifications Reference

**ACTION:** Replace line 161 placeholder with 28 integration entries

### 2. Cloud-Dispatched Integrations
Insert entries **between line 164 and line 166**

Current content around that area:
- Line 163: Manage notification methods
- Line 164: Manage alert notification silencing rules
- **Line 165: PLACEHOLDER** `cloud_notifications_integrations,,,,,`
- Line 166: Alert Configuration Reference

**ACTION:** Replace line 165 placeholder with 15 integration entries

---

## FILES TO ADD (in learn/ folder)

### Agent-Dispatched Integrations (28 total)
Located in: `learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/`

| # | Integration | Filename |
|---|------------|----------|
| 1 | alerta | alerta.mdx |
| 2 | aws-sns | aws-sns.mdx |
| 3 | custom | custom.mdx |
| 4 | discord | discord.mdx |
| 5 | dynatrace | dynatrace.mdx |
| 6 | email | email.mdx |
| 7 | flock | flock.mdx |
| 8 | gotify | gotify.mdx |
| 9 | ilert | ilert.mdx |
| 10 | irc | irc.mdx |
| 11 | kavenegar | kavenegar.mdx |
| 12 | matrix | matrix.mdx |
| 13 | microsoft-teams | microsoft-teams.mdx |
| 14 | messagebird | messagebird.mdx |
| 15 | ntfy | ntfy.mdx |
| 16 | opsgenie | opsgenie.mdx |
| 17 | pagerduty | pagerduty.mdx |
| 18 | prowl | prowl.mdx |
| 19 | pushbullet | pushbullet.mdx |
| 20 | pushover | pushover.mdx |
| 21 | rocketchat | rocketchat.mdx |
| 22 | signal4 | signal4.mdx |
| 23 | slack | slack.mdx |
| 24 | smseagle | smseagle.mdx |
| 25 | sms | sms.mdx |
| 26 | syslog | syslog.mdx |
| 27 | telegram | telegram.mdx |
| 28 | twilio | twilio.mdx |

### Cloud-Dispatched Integrations (15 total)
Located in: `learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/`

| # | Integration | Filename |
|---|------------|----------|
| 1 | amazon-sns | amazon-sns.mdx |
| 2 | discord | discord.mdx |
| 3 | ilert | ilert.mdx |
| 4 | mattermost | mattermost.mdx |
| 5 | microsoft-teams | microsoft-teams.mdx |
| 6 | netdata-mobile-app | netdata-mobile-app.mdx |
| 7 | opsgenie | opsgenie.mdx |
| 8 | pagerduty | pagerduty.mdx |
| 9 | rocketchat | rocketchat.mdx |
| 10 | servicenow | servicenow.mdx |
| 11 | slack | slack.mdx |
| 12 | splunk | splunk.mdx |
| 13 | splunk-victorops | splunk-victorops.mdx |
| 14 | telegram | telegram.mdx |
| 15 | webhook | webhook.mdx |

---

## ENTRY FORMAT

### Agent Example (email.mdx)
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/email.mdx,Email,Published,Alerts & Notifications/Notifications/Agent Dispatched Notifications,
```

### Cloud Example (slack.mdx)
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/slack.mdx,Slack,Published,Alerts & Notifications/Notifications/Centralized Cloud Notifications,
```

**Parts breakdown:**
- Column 1: Edit URL pointing to file in learn/ folder
- Column 2: Display name (capitalized integration name)
- Column 3: Published
- Column 4: Hierarchical path (NO "The Book/" - flat structure)
- Column 5+: Optional description

---

## WHAT EXISTS ALREADY (DO NOT MODIFY)

- All 12 chapters under docs/alerts/ are already mapped (lines 169-226)
- Headers and references for Notifications section (lines 159-164)
- Learning AI, Insights sections (later in file)

---

## STEPS

1. Open `docs/.map/map.csv`
2. Find line 161 (`agent_notifications_integrations`) → replace with 28 Agent entries
3. Find line 165 (`cloud_notifications_integrations`) → replace with 15 Cloud entries
4. Save and verify