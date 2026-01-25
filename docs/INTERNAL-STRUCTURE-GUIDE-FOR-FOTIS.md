# Internal Guide: Alerts & Notifications Structure (DO NOT PUBLISH)

**For:** Fotis
**Purpose:** Map CSV structure updates for learn/ documentation

---

## What Fotis Needs to Do

**ONLY update `docs/.map/map.csv`** - nothing else. Focus on populating the two placeholder rows:

| Line | Placeholder | What to Add |
|------|-----------|-------------|
| ~161 | `agent_notifications_integrations` | Agent-dispatched integration entries |
| ~165 | `cloud_notifications_integrations` | Cloud-dispatched integration entries |

---

## Current Structure in map.csv

### Existing Alert Documentation (Already Done)
See `map.csv` lines ~158-226 for the complete 12-chapter structure under `Alerts & Notifications/`.

### Placeholders to Populate

**Line ~161: Agent Notifications (Agent-DISPATCHED)**
```
agent_notifications_integrations,,,,,
```
↓ Add entries for each Agent integration here

**Line ~165: Cloud Notifications (Cloud-DISPATCHED)**
```
cloud_notifications_integrations,,,,,
```
↓ Add entries for each Cloud integration here

---

## Source: Individual Integration Files in learn/

All `.mdx` files already exist. Count them and populate corresponding map.csv entries.

### Agent-DISPATCHED Integrations
Location: `learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/`

**Main files:**
- `agent-dispatched-notifications.mdx`
- `agent-notifications-reference.mdx`

**Individual integrations (24 total):**
| alerta | aws-sns | custom | discord | dynatrace |
|--------|---------|--------|---------|----------|
| email | flock | gotify | ilert | irc |
| kavenegar | matrix | microsoft-teams | messagebird | ntfy |
| opsgenie | pagerduty | prowl | pushbullet | pushover |
| rocketchat | signal4 | slack | smseagle | sms |
| syslog | telegram | twilio | | |

### Cloud-DISPATCHED Integrations
Location: `learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/`

**Main files:**
- `centralized-cloud-notifications.mdx`
- `centralized-cloud-notifications-reference.mdx`
- `manage-notification-methods.mdx`
- `manage-alert-notification-silencing-rules.mdx`

**Individual integrations (15 total):**
| amazon-sns | discord | ilert | mattermost | microsoft-teams |
|-------------|---------|-------|------------|----------------|
| netdata-mobile-app | opsgenie | pagerduty | rocketchat | servicenow |
| slack | splunk | splunk-victorops | telegram | webhook |

---

## Pattern for Integration Entries

### Example: Agent Email Integration
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/agent-dispatched-notifications/email.mdx,Email,Published,Alerts & Notifications/Notifications/Agent Dispatched Notifications,
```

### Example: Cloud Slack Integration
```
https://github.com/netdata/netdata/edit/master/learn/docs/alerts-&-notifications/notifications/centralized-cloud-notifications/slack.mdx,Slack,Published,Alerts & Notifications/Notifications/Centralized Cloud Notifications,
```

**Key fields:**
- `learn_rel_path`: `Alerts & Notifications/Notifications/Agent Dispatched Notifications` OR `Alerts & Notifications/Notifications/Centralized Cloud Notifications`

---

## What NOT Included in Old Structure

The old `docs/alerts-and-notifications/` directory content is intentionally excluded from this map.csv update since we're building fresh structure in `learn/`.

---

## Questions?

Contact the documentation team for clarification. Do not cross-reference source code for terminology - that's handled separately.