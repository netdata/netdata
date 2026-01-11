# 13.4 Notification Dispatch Architecture

When an alert fires, the notification system handles delivery to configured destinations.

## Notification Queue

Alert events enter a notification queue for asynchronous delivery. This queuing prevents alert evaluation from blocking on slow notification destinations.

Queue overflow causes event loss. When the queue fills, new events are discarded. Monitor queue utilization to detect delivery problems.

## Notification Methods

Netdata supports multiple notification methods including email, Slack, PagerDuty, Discord, Telegram, and others. Each method has a dedicated handler that formats and delivers notifications.

Configuration files in `/etc/netdata/` define notification settings. The `health_alarm_notify.conf` file configures method-specific settings.

## Delivery Reliability

Notification delivery is best-effort with configurable retry behavior. Failed delivery attempts are logged for troubleshooting.

Critical notification delivery should use redundant paths. Configure multiple notification methods for critical alerts.

## Escalation and Routing

Notification routing determines which recipients receive which alerts. Routing rules can filter by alert name, chart, host, or severity.

Escalation policies route unacknowledged alerts to secondary recipients after timeout periods.