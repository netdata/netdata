# 12.2 Notification Strategy and On-Call Hygiene

Alert configuration and notification configuration are independent concerns. An alert can fire correctly yet never reach the right people if notifications are misconfigured.

:::tip
Define clear ownership for every alert. When an alert fires, who receives it? If multiple people could respond, who should lead?
:::

## 12.2.1 Routing Alerts to the Right People

Alerts should reach the people best positioned to respond. Routing alerts to generic distribution lists or channels that no one monitors defeats the purpose of alerting.

| Routing Factor | Consideration |
|---------------|---------------|
| **Ownership** | Who owns this service/component? |
| **Time zone** | Which team handles this during off-hours? |
| **Escalation** | Who receives unacknowledged alerts? |
| **Language** | Are responders comfortable with the alert language? |

Consider time-of-day routing. Alerts requiring response during business hours may not need overnight paging. Use scheduled routing to deliver warnings during business hours and critical alerts around the clock.

## 12.2.2 On-Call Hygiene

Effective on-call rotation design prevents burnout while ensuring coverage. Rotate responsibilities regularly to distribute load across the team.

| On-Call Best Practice | Recommendation |
|----------------------|----------------|
| **Rotation frequency** | Every 4-5 weeks between rotations |
| **Response time** | 15-minute acknowledgment for critical |
| **Escalation timeout** | Automatic after 15 minutes |
| **Tool access** | Pre-configured jump boxes, dashboards |

Define clear escalation paths. When on-call responders receive an alert, they should know immediately whether to respond or escalate. Escalation should be automatic after timeout periods rather than requiring manual decision-making.

Provide adequate tools and access. On-call responders need enough context to begin investigation immediately. Alert messages should include diagnostic links, relevant metrics, and known remediation steps.

## 12.2.3 Avoiding Notification Overload

The same alert firing repeatedly is noise. Configure repeat intervals to prevent notification storms for sustained conditions.

Use escalation policies to manage notification intensity. An initial critical alert pages the primary responder; if unacknowledged after fifteen minutes, the secondary responder receives a page.

Silence rules should be explicit and documented. Planned maintenance windows, scheduled deployments, and known issues should have corresponding silence rules. But silence rules should expire automatically and never be used as a substitute for fixing alerts that fire inappropriately.

## 12.2.4 Severity and Urgency Matching

Match notification method to alert severity and urgency. Critical alerts warrant immediate paging. Warning alerts may wait for business hours. Informational alerts may simply appear in dashboards.

| Severity | Notification Method | Expected Response |
|----------|---------------------|-------------------|
| CRIT | Paging, SMS | Immediate (< 15 min) |
| WARN | Email, Slack | Same business day |
| INFO | Dashboard | Review during normal work |

Consider the time-sensitivity of each alert type. An alert requiring response within minutes should use different notification methods than an alert requiring response within days.

Avoid using the same notification channel for alerts with vastly different urgencies. Paging for critical issues loses effectiveness when mixed with informational notifications.

## 12.2.5 Post-Incident Review

After responding to alerts, conduct brief reviews to improve future responses. What information would have helped resolve faster? What alerts were false positives?

Update alert configurations based on response patterns. Alerts that consistently trigger false positives need threshold adjustments. Alerts that require investigation before responders can act need better documentation.

## What's Next

- [12.3 Maintaining Configurations](3-maintaining-configurations.md) - Version control and periodic reviews
- [12.4 Large Environment Patterns](4-scaling-large-environments.md) - Parent-based and distributed setups
- [12.5 SLI and SLO Alerts](5-sli-slo-alerts.md) - Connecting to business objectives
- [13. Alerts and Notifications Architecture](../architecture/index.md) - Internal behavior

## See Also

- [Receiving Notifications](../../receiving-notifications/index.md) - Notification configuration options
- [Silencing Rules](../../cloud-alert-features/silencing-rules.md) - Cloud silencing rules
- [Reducing Flapping and Noise](../../controlling-alerts-noise/reducing-flapping.md) - Managing alert frequency