# Centralized Cloud Notifications Reference

Netdata Cloud sends alert notifications for nodes in warning, critical, or unreachable states, ensuring alerts are managed centrally and efficiently.

## Benefits of Centralized Notifications

- Consolidate health status views across all infrastructure in one place.
- Set up and [manage your alert notifications easily](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md).
- Expedite troubleshooting with tools like [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md) and the [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboards-and-charts/anomaly-advisor-tab.md).

To avoid notification overload, **flood protection** is triggered when a node frequently disconnects or sends excessive alerts, highlighting potential issues.

Admins must [enable alert notifications](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized-cloud-notifications/manage-notification-methods.md#manage-space-notification-settings) for their Space(s). All users can then customize their notification preferences through their [account menu](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/#manage-user-notification-settings).

> **Note**
>
> Centralized alerts in Netdata Cloud are separate from the [Netdata Agent](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/README.md) notifications. Alerts must be [configured individually](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) on each node.

## Alert Notifications

Notifications can be sent via email or through third-party services like PagerDuty or Slack. Admins manage notification settings for the entire Space, while individual users can personalize settings in their profile.

### Service Level

#### Personal

Notifications are sent to user-specific destinations, such as email, which are managed by users under their profile settings.

#### System

These notifications go to general targets like a Slack channel, with admins setting rules for notification targets based on workspace or alert level.

### Service Classification

#### Community

Available to all plans, includes basic methods like Email and Discord.

#### Business

Exclusive to [paid plans](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md), includes advanced services like PagerDuty and Slack.

## Alert Notification Silencing Rules

Netdata Cloud offers a silencing rule engine to mute alert notifications based on specific conditions related to nodes or alert types. Learn how to manage these settings [here](https://github.com/netdata/netdata/blob/master/docs/alerts-and-notifications/notifications/centralized/cloud/notifications/manage-alert-notification-silending-rules.md).

## Flood Protection

If a node repeatedly changes state or fires alerts, flood protection limits notifications to prevent overload. You can still access node details through Netdata Cloud or directly via the local Agent dashboard.

## Anatomy of an Email Alert Notification

Email notifications provide comprehensive details:

- The Space's name
- The node's name
- Alert status: critical, warning, cleared
- Previous alert status
- Time at which the alert triggered
- Chart context that triggered the alert
- Name and information about the triggered alert
- Alert value
- Total number of warning and critical alerts on that node
- Threshold for triggering the given alert state
- Calculation or database lookups that Netdata uses to compute the value
- Source of the alert, including which file you can edit to configure this alert on an individual node
- Direct link to the node’s chart in Cloud dashboards.
