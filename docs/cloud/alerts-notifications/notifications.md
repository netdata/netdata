# Centralized Cloud Notifications Reference

Netdata Cloud can send centralized alert notifications to your team whenever a node enters a warning, critical, or unreachable state. By enabling notifications, you ensure that no alert on any node in your infrastructure goes unnoticed by you or your team.

Having this information centralized helps you:

* Have a clear view of the health across your infrastructure, seeing all alerts in one place.
* Easily [set up your alert notification process](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md):
methods to use and where to use them, filtering rules, etc.
* Quickly troubleshoot using [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
or [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboard/anomaly-advisor-tab.md)

To prevent alert overload, we automatically send a "flood protection" notification if a node experiences frequent disconnections or triggers a high volume of alerts. This notification highlights potential health or performance problems on that specific node.

Admins must [enable alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-space-notification-settings) for their Space(s). All users in the Space can then personalize their notifications settings in [account menu](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/#manage-user-notification-settings).

> **Note**
>
> Centralized alerts in Netdata Cloud work independently of notifications sent by individual [Netdata Agents](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md). You can use one or both depending on your needs. Сentralized alerts displayed in Netdata Cloud come directly from your Netdata-monitored nodes. To customize or add new alerts, you need to [configure them](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) on each node.

## Alert notifications

Alert notifications are delivered through various methods, including email sent directly from Netdata and integrations with third-party services like PagerDuty or Slack.

Notification methods are classified on two main attributes:

* Service level: Personal or System
* Service classification: Community or Business

Only administrators are able to manage the space's alert notification settings.
All users in a Space can personalize their notifications settings, for Personal service level notification methods, from within their profile menu.

> **Note**
>
> Netdata Cloud supports various notification methods, and their accessibility may vary depending on your subscription plan. For more details check [Service classification](#service-classification) and [netdata.cloud/pricing](https://www.netdata.cloud/pricing).

### Service level

#### Personal

The notifications methods classified as **Personal** are what we consider generic, meaning that these can't have specific rules for them set by the administrators.

Notifications are sent to user-defined destinations, such as email addresses. Users can manage their notification preferences, including Space/Room configurations and desired notification level, from the **Notifications** section of their User Profile.

One example of such a notification method is the E-mail.

#### System

For **System** notification methods, the destination of the channel will be a target that usually isn't specific to a single user, e.g. slack channel.

Administrators can define granular notification rules for these methods. This allows for flexible configurations, where you can specify different notification targets based on Space/Room or desired notification level.

Some examples of such notification methods are: Webhook, PagerDuty, Slack.

### Service classification

#### Community

Notification methods classified as Community can be used by everyone independent on the plan your space is at.
These are: Email and discord

#### Business

Notification methods classified as Business are only available for [paid plans](https://github.com/netdata/netdata/blob/master/docs/cloud/manage/plans.md)

These are: PagerDuty, Slack, Opsgenie

## Alert notifications silencing rules

Netdata Cloud provides you a silencing Rule engine which allows you to mute alert notifications. This muting action is specific to alert state transition notifications, it doesn't include node unreachable state transitions.

The silencing rule engine is flexible and allows you to enter silence rules for the two main entities involved on alert notifications and can be set using different attributes. The main entities you can enter are **Nodes** and **Alerts** which can be used in combination or isolation to target specific needs. Learn how to [manage alert notification silencing rules](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-alert-notification-silencing-rules.md).

## Flood protection

If a node has too many state changes like firing too many alerts or going from reachable to unreachable, Netdata Cloud enables flood protection. As long as a node is in flood protection mode, Netdata Cloud does not send notifications about this node. Even with flood protection active, it is possible to access the node directly, either via Netdata Cloud or the local Agent dashboard at `http://NODEIP:19999`.

## Anatomy of an email alert notification

Email alert notifications show the following information:

* The Space's name
* The node's name
* Alert status: critical, warning, cleared
* Previous alert status
* Time at which the alert triggered
* Chart context that triggered the alert
* Name and information about the triggered alert
* Alert value
* Total number of warning and critical alerts on that node
* Threshold for triggering the given alert state
* Calculation or database lookups that Netdata uses to compute the value
* Source of the alert, including which file you can edit to configure this alert on an individual node

Email notifications also feature a **Go to Node** button, which takes you directly to the offending chart for that node within Cloud's embedded dashboards.
