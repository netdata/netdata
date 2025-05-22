# Notifications

Netdata supports two ways to send alert notifications: **from Netdata Cloud** or **from the Netdata Agent**.

You can use either or both depending on how your infrastructure is set up.

:::tip

Need alerts *fast*?  
Use Cloud for a centralized setup, or Agent for full control on each node.

:::

## Notification Methods

### Netdata Cloud (Centralized)

Netdata Cloud collects alert data from all connected nodes and sends notifications through your configured integrations.

[See supported Cloud integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/centralized-cloud-notifications)

**Popular integrations:**

- Amazon SNS
- Slack
- Discord
- Splunk
- Microsoft Teams
- Mattermost

### Netdata Agent (Local)

The Agent sends alerts directly from the node, even if it's offline or not connected to the Cloud.

[See supported Agent integrations](https://learn.netdata.cloud/docs/alerts-&-notifications/notifications/agent-dispatched-notifications)

**Popular integrations:**

- Email
- Slack
- PagerDuty
- Twilio
- Telegram
- Opsgenie

## Alert Intelligence and Severity Levels

Netdata's alerts are designed to minimize false positives and prevent alarm fatigue.

### Alert Severity Levels

| Level        | Description                                                       | Typical Action                    |
|--------------|-------------------------------------------------------------------|-----------------------------------|
| **CLEAR**    | The metric has returned to normal range                           | No action needed                  |
| **WARNING**  | The metric shows concerning behavior that requires attention      | Investigate during business hours |
| **CRITICAL** | The metric indicates a serious problem requiring immediate action | Immediate response required       |

These severity levels help you prioritize your response and can be routed to different notification channels based on urgency.

### Preventing Alert Fatigue

| Feature                   | Benefit                                                               |
|---------------------------|-----------------------------------------------------------------------|
| **Intelligent Defaults**  | Thresholds carefully selected based on real-world experience          |
| **Hysteresis Protection** | Prevents notification floods when metrics fluctuate around thresholds |
| **Notification Delays**   | Configurable delays ensure transient issues don't trigger alerts      |
| **Role-Based Routing**    | Ensures alerts reach only the appropriate stakeholders                |

:::tip

You can configure different notification channels for different severity levels. For example, you can send WARNING alerts to Slack and CRITICAL alerts to PagerDuty.

:::

## Troubleshooting Assistance

When you receive an alert, Netdata provides tools to help you understand and resolve the issue:

### Netdata Assistant

The [Netdata Assistant](https://learn.netdata.cloud/docs/machine-learning-and-anomaly-detection/ai-powered-troubleshooting-assistant) is an AI-powered feature that guides you through troubleshooting alerts by providing:

- Clear explanations of what the alert means
- Assessment of potential causes
- Recommended troubleshooting steps
- Links to relevant documentation

The Assistant window follows you as you navigate through dashboards, making troubleshooting faster and more efficient.

### Community Resources

For more complex issues, you can access the [Alerts Troubleshooting space](https://community.netdata.cloud/c/alerts/28) in our community forum, where you'll find:

- Detailed information about all built-in alerts
- Recommended troubleshooting actions from experts
- A searchable history of previously solved issues
- Ability to ask questions and share your own solutions

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

## Next Steps

- 🔧 [Set up Cloud Notifications](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/centralized-cloud-notifications-reference.md)
- ⚙️ [Set up Agent Notifications](/src/health/notifications/README.md)

:::info

For additional alert customization options including threshold adjustments, custom conditions, and notification routing, check out our [alert configuration reference](https://learn.netdata.cloud/docs/alerts-&-notifications/alert-configuration-reference).

:::
