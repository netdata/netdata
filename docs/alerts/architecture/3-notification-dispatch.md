# 13.3 Notification Dispatch Architecture

When an alert fires, the notification system handles delivery to configured destinations.

:::note

Notification delivery is asynchronous. Queue overflow can cause event loss. Monitor queue utilization for production systems.

:::

## 13.3.1 Notification Queue

Alert events enter a notification queue for asynchronous delivery. This queuing prevents alert evaluation from blocking on slow notification destinations.

Queue overflow causes event loss. When the queue fills, new events are discarded. Monitor queue utilization to detect delivery problems.

| Queue State | Behavior |
|-------------|----------|
| **Normal** | Events processed within milliseconds |
| **Filling** | Processing slower than arrival rate |
| **Overflow** | New events discarded |

## 13.3.2 Notification Methods

Netdata supports multiple notification methods including email, Slack, PagerDuty, Discord, Telegram, and others. Each method has a dedicated handler that formats and delivers notifications.

| Method | Use Case |
|--------|----------|
| **Email** | Non-urgent notifications, summaries |
| **Slack** | Team channels, rapid response |
| **PagerDuty** | Critical alerts, on-call routing |
| **Discord** | Community monitoring, quick alerts |
| **Telegram** | Direct messaging, bot integration |

Configuration files in `/etc/netdata/` define notification settings. The `health_alarm_notify.conf` file configures method-specific settings.

## 13.3.3 Delivery Reliability

Notification delivery is best-effort with configurable retry behavior. Failed delivery attempts are logged for troubleshooting.

Critical notification delivery should use redundant paths. Configure multiple notification methods for critical alerts.

## 13.3.4 Escalation and Routing

Notification routing determines which recipients receive which alerts. Routing rules can filter by alert name, chart, host, or severity.

Escalation policies route unacknowledged alerts to secondary recipients after timeout periods.

| Routing Factor | Description |
|----------------|-------------|
| **Alert name** | Match specific alerts |
| **Chart/context** | Filter by resource type |
| **Host** | Scope to specific nodes |
| **Severity** | Route by warning/critical |

## Related Sections

- [13.1 Evaluation Architecture](./1-evaluation-architecture.md) - Alert evaluation process
- [13.5 Scaling Topologies](./5-scaling-topologies.md) - Distributed notification handling
- [Receiving Notifications](../../receiving-notifications/index.md) - Complete notification guide