# Centralized Cloud Notifications Reference

Netdata Cloud can send centralized alert notifications to your team whenever a node enters a warning, critical, or unreachable state. By enabling notifications, you ensure no alert, on any node in your infrastructure, goes unnoticed by you or your team.

Having this information centralized helps you:

* Have a clear view of the health across your infrastructure, seeing all alerts in one place.
* Easily [set up your alert notification process](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md):
methods to use and where to use them, filtering rules, etc.
* Quickly troubleshoot using [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
or [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/dashboard/anomaly-advisor-tab.md)

If a node is getting disconnected often or has many alerts, we protect you and your team from alert fatigue by sending you a flood protection notification. Getting one of these notifications is a good signal of health or performance issues on that node.

Admins must [enable alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-space-notification-settings) for their Space(s). All users in a Space can then personalize their notifications settings from within their [account menu](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/#manage-user-notification-settings).

> **Note**
>
> Centralized alert notifications from Netdata Cloud is a independent process from [notifications from Netdata Agents](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md). You can enable one or the other, or both, based on your needs. However, the alerts you see in Netdata Cloud are based on those streamed from your Netdata-monitoring nodes. If you want to tweak or add new alert that you see in Netdata Cloud, and receive via centralized alert notifications, you must [configure](https://github.com/netdata/netdata/blob/master/src/health/REFERENCE.md) each node's alert watchdog.

## Alert notifications

Alert notifications can be delivered through different methods, these can go from an Email sent from Netdata to the use of a 3rd party integration like PagerDuty or Slack.

Notification methods are classified on two main attributes:

* Service level: Personal or System
* Service classification: Community or Business

Only administrators are able to manage the space's alert notification settings.
All users in a Space can personalize their notifications settings, for Personal service level notification methods, from within their profile menu.

> **Note**
>
> Netdata Cloud supports different notification methods and their availability will depend on the plan you are at. For more details check [Service classification](#service-classification) and [netdata.cloud/pricing](https://www.netdata.cloud/pricing).

### Service level

#### Personal

The notifications methods classified as **Personal** are what we consider generic, meaning that these can't have specific rules for them set by the administrators.

These notifications are sent to the destination of the channel which is a user-specific attribute, e.g. user's e-mail, and the users are the ones that will then be able to manage what specific configurations they want for the Space / Room(s) and the desired Notification level, they can achieve this from their User Profile page under **Notifications**.

One example of such a notification method is the E-mail.

#### System

For **System** notification methods, the destination of the channel will be a target that usually isn't specific to a single user, e.g. slack channel.

These notification methods allow for fine-grain rule settings to be done by administrators and more than one configuration can exist for them since. You can specify different targets depending on Room or Notification level settings.

Some examples of such notification methods are: Webhook, PagerDuty, Slack.

### Service classification

#### Community

Notification methods classified as Community can be used by everyone independent on the plan your space is at.
These are: Email and discord

#### Business

Notification methods classified as Business are only available for **Business** plans
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
