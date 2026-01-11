# 12.3 Notification Strategy

Alert configuration and notification configuration are independent concerns. An alert can fire correctly yet never reach the right people if notifications are misconfigured.

## Routing Alerts to the Right People

Alerts should reach the people best positioned to respond. Routing alerts to generic distribution lists or channels that no one monitors defeats the purpose of alerting.

Define clear ownership for every alert. When an alert fires, who receives it? If multiple people could respond, who should lead?

Consider time-of-day routing. Alerts requiring response during business hours may not need overnight paging. Use scheduled routing to deliver warnings during business hours and critical alerts around the clock.

## Avoiding Notification Overload

The same alert firing repeatedly is noise. Configure repeat intervals to prevent notification storms for sustained conditions.

Use escalation policies to manage notification intensity. An initial critical alert pages the primary responder; if unacknowledged after fifteen minutes, the secondary responder receives a page.

Silence rules should be explicit and documented. Planned maintenance windows, scheduled deployments, and known issues should have corresponding silence rules. But silence rules should expire automatically and never be used as a substitute for fixing alerts that fire inappropriately.