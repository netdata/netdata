# Centralized Cloud Notifications Reference

Netdata Cloud sends Alert notifications for nodes in warning, critical, or unreachable states, ensuring Alerts are managed centrally and efficiently.

## Benefits of Centralized Notifications

- Consolidate health status views across all infrastructure in one place.
- Set up and [manage your Alert notifications easily](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md).
- Expedite troubleshooting with tools like [Metric Correlations](/docs/metric-correlations.md) and the [Anomaly Advisor](/docs/dashboards-and-charts/anomaly-advisor-tab.md).

> **Note**
>
> To avoid notification overload, **flood protection** is triggered when a node frequently disconnects or sends excessive Alerts, highlighting potential issues.

Administrators must [enable Alert notifications](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-space-notification-settings) for their Space(s). All users can then customize their notification preferences through their [account menu](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-user-notification-settings).

> **Note**
>
> Centralized Alerts in Netdata Cloud are separate from the [Netdata Agent](/docs/alerts-and-notifications/notifications/README.md) notifications. Agent Alerts must be [configured individually](/src/health/REFERENCE.md) on each node.

## Alert Notifications

Notifications can be sent via email or through third-party services like PagerDuty or Slack. Administrators can manage notification settings for the entire Space, while individual users can personalize settings in their profile.

### Service Level

#### Personal

Notifications are sent to user-specific destinations, such as email, which are managed by users under their profile settings.

#### System

These notifications go to general targets like a Slack channel, with administrators setting rules for notification targets based on workspace or Alert level.

### Service Classification

#### Community

Available to all plans, includes basic methods like Email and Discord.

#### Business

Exclusive to [paid plans](/docs/netdata-cloud/view-plan-and-billing.md), includes advanced services like PagerDuty and Slack.

## Alert Notification Silencing Rules

Netdata Cloud offers a silencing rule engine to mute Alert notifications based on specific conditions related to nodes or Alert types. Learn how to manage these settings [here](/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-alert-notification-silencing-rules.md).

## Flood Protection

If a node repeatedly changes state or raises Alerts, flood protection limits notifications to prevent overload. You can still access node details through Netdata Cloud or directly via the local Agent dashboard.

## Anatomy of an Email Alert Notification

Email notifications provide comprehensive details:

- The Space's name
- The node's name
- Alert status: critical, warning, cleared
- Previous Alert status
- Time at which the Alert triggered
- Chart context that triggered the Alert
- Name and information about the triggered Alert
- Alert value
- Total number of warning and critical Alerts on that node
- Threshold for triggering the given Alert state
- Calculation or database lookups that Netdata uses to compute the value
- Source of the Alert, including which file you can edit to configure this Alert on an individual node
- Direct link to the node’s chart in Cloud dashboards.
