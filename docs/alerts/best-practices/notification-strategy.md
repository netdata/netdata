# 12.2 Notification Strategy and On-Call Hygiene

Alert configuration and notification configuration are independent concerns. An alert can fire correctly yet never reach the right people if notifications are misconfigured.

## Routing Alerts to the Right People

Alerts should reach the people best positioned to respond. Routing alerts to generic distribution lists or channels that no one monitors defeats the purpose of alerting.

Define clear ownership for every alert. When an alert fires, who receives it? If multiple people could respond, who should lead?

Consider time-of-day routing. Alerts requiring response during business hours may not need overnight paging. Use scheduled routing to deliver warnings during business hours and critical alerts around the clock.

## On-Call Hygiene

Effective on-call rotation design prevents burnout while ensuring coverage. Rotate responsibilities regularly to distribute load across the team.

Schedule reasonable on-call frequency. Someone on call every other week has inadequate recovery time. Every fourth or fifth week provides better balance.

Define clear escalation paths. When on-call responders receive an alert, they should know immediately whether to respond or escalate. Escalation should be automatic after timeout periods rather than requiring manual decision-making.

Provide adequate tools and access. On-call responders need enough context to begin investigation immediately. Alert messages should include diagnostic links, relevant metrics, and known remediation steps.

## Avoiding Notification Overload

The same alert firing repeatedly is noise. Configure repeat intervals to prevent notification storms for sustained conditions.

Use escalation policies to manage notification intensity. An initial critical alert pages the primary responder; if unacknowledged after fifteen minutes, the secondary responder receives a page.

Silence rules should be explicit and documented. Planned maintenance windows, scheduled deployments, and known issues should have corresponding silence rules. But silence rules should expire automatically and never be used as a substitute for fixing alerts that fire inappropriately.

## Severity and Urgency Matching

Match notification method to alert severity and urgency. Critical alerts warrant immediate paging. Warning alerts may wait for business hours. Informational alerts may simply appear in dashboards.

Consider the time-sensitivity of each alert type. An alert requiring response within minutes should use different notification methods than an alert requiring response within days.

Avoid using the same notification channel for alerts with vastly different urgencies. Paging for critical issues loses effectiveness when mixed with informational notifications.

## Post-Incident Review

After responding to alerts, conduct brief reviews to improve future responses. What information would have helped resolve faster? What alerts were false positives?

Update alert configurations based on response patterns. Alerts that consistently trigger false positives need threshold adjustments. Alerts that require investigation before responders can act need better documentation.

## What's Next

- **12.3 Maintaining Alert Configurations** for version control and periodic reviews
- **12.4 Large Environment Patterns** for parent-based and distributed setups
- **12.5 SLIs and SLOs** for connecting to business objectives
- **13. Alerts and Notifications Architecture** for internal behavior