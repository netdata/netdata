# Notifications

Netdata supports two ways to send alert notifications: **from Netdata Cloud** or **from the Netdata Agent**.  
You can use either—or both—depending on how your infrastructure is set up.

:::tip Need alerts fast?
Use Cloud for a centralized setup, or Agent for full control on each node.
:::

---

## Notification Methods

### Netdata Cloud (Centralized)

Netdata Cloud collects alert data from all connected nodes and sends notifications through your configured integrations.

→ [See supported Cloud integrations](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications)

**Popular integrations:**

- Amazon SNS
- Slack
- Discord
- Splunk
- Microsoft Teams

---

### Netdata Agent (Local)

The Agent sends alerts directly from the node, even if it's offline or not connected to the Cloud.

→ [See supported Agent integrations](/docs/alerts-and-notifications/notifications/agent-dispatched-notifications)

**Popular integrations:**

- Email
- Slack
- PagerDuty
- Twilio
- Telegram
- Opsgenie

---

## Which One Should I Use?

Choose the option that fits your needs:

| Use Case                              | Best Option   |
|---------------------------------------|---------------|
| Manage multiple nodes centrally       | Netdata Cloud |
| Fewer configs, alerts from one place  | Netdata Cloud |
| Full control at node level            | Netdata Agent |
| No internet or external dependencies  | Netdata Agent |
| Fine-tuned control per system/service | Netdata Agent |
| Want both simplicity and flexibility  | Use **both**  |

---

## Next Steps

- 🔧 [Set up Cloud Notifications](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications)
- ⚙️ [Set up Agent Notifications](/docs/alerts-and-notifications/notifications/agent-dispatched-notifications)

:::info Want help with alert customization?
You can tune thresholds, write custom conditions, and control who gets notified.
[Learn more here →](/src/health/REFERENCE.md)
:::