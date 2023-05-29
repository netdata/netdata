# Cloud alert notifications

import Callout from '@site/src/components/Callout'

Netdata Cloud can send centralized alert notifications to your team whenever a node enters a warning, critical, or
unreachable state. By enabling notifications, you ensure no alert, on any node in your infrastructure, goes unnoticed by
you or your team.

Having this information centralized helps you:
* Have a clear view of the health across your infrastructure, seeing all alerts in one place.
* Easily [setup your alert notification process](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md): 
methods to use and where to use them, filtering rules, etc.
* Quickly troubleshoot using [Metric Correlations](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/metric-correlations.md)
or [Anomaly Advisor](https://github.com/netdata/netdata/blob/master/docs/cloud/insights/anomaly-advisor.md)

If a node is getting disconnected often or has many alerts, we protect you and your team from alert fatigue by sending
you a flood protection notification. Getting one of these notifications is a good signal of health or performance issues
on that node.

Admins must enable alert notifications for their [Space(s)](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-notification-methods.md#manage-space-notification-settings). All users in a
Space can then personalize their notifications settings from within their [account
menu](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/#manage-user-notification-settings).

<Callout type="notice">

Centralized alert notifications from Netdata Cloud is a independent process from [notifications from
Netdata](https://github.com/netdata/netdata/blob/master/docs/monitor/enable-notifications.md). You can enable one or the other, or both, based on your needs. However,
the alerts you see in Netdata Cloud are based on those streamed from your Netdata-monitoring nodes. If you want to tweak
or add new alert that you see in Netdata Cloud, and receive via centralized alert notifications, you must
[configure](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) each node's alert watchdog.

</Callout>

## Alert notifications

Netdata Cloud can send centralized alert notifications to your team whenever a node enters a warning, critical, or unreachable state. By enabling notifications, 
you ensure no alert, on any node in your infrastructure, goes unnoticed by you or your team.

If a node is getting disconnected often or has many alerts, we protect you and your team from alert fatigue by sending you a flood protection notification. 
Getting one of these notifications is a good signal of health or performance issues on that node.

Alert notifications can be delivered through different methods, these can go from an Email sent from Netdata to the use of a 3rd party tool like PagerDuty.

Notification methods are classified on two main attributes:
* Service level: Personal or System
* Service classification: Community or Business

Only administrators are able to manage the space's alert notification settings.
All users in a Space can personalize their notifications settings, for Personal service level notification methods, from within their profile menu.

> ⚠️ Netdata Cloud supports different notification methods and their availability will depend on the plan you are at.
> For more details check [Service classification](#service-classification) or [netdata.cloud/pricing](https://www.netdata.cloud/pricing).

### Service level

#### Personal

The notifications methods classified as **Personal** are what we consider generic, meaning that these can't have specific rules for them set by the administrators.

These notifications are sent to the destination of the channel which is a user-specific attribute, e.g. user's e-mail, and the users are the ones that will then be able to
manage what specific configurations they want for the Space / Room(s) and the desired Notification level, they can achieve this from their User Profile page under 
**Notifications**.

One example of such a notification method is the E-mail.

#### System

For **System** notification methods, the destination of the channel will be a target that usually isn't specific to a single user, e.g. slack channel.

These notification methods allow for fine-grain rule settings to be done by administrators and more than one configuration can exist for them since. You can specify 
different targets depending on Rooms or Notification level settings.

Some examples of such notification methods are: Webhook, PagerDuty, Slack.

### Service classification

#### Community

Notification methods classified as Community can be used by everyone independent on the plan your space is at.
These are: Email and discord

#### Pro

Notification methods classified as Pro are only available for **Pro** and **Business** plans
These are: webhook

#### Business

Notification methods classified as Business are only available for **Business** plans
These are: PagerDuty, Slack, Opsgenie

## Silencing Alert notifications

Netdata Cloud provides you a Silencing Rule engine which allows you to mute alert notifications. This muting action is specific to alert state transition notifications, it doesn't include node unreachable state transitions.

The Silencing Rule engine is flexible and allows you to enter silence rules for the two main entities involved on alert notifications and can be set using different attributes. The main entities you can enter are **Nodes** and **Alerts** which can be used in combination or isolation to target specific needs - see some examples [here](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-alert-notification-silencing-rules.md#silencing-rules-examples).

### Scope definition for Nodes
* **Space:** silencing the space, selecting `All Rooms`, silences all alert state transitions from any node claimed to the space.
* **War Room:** silencing a specific room will silence all alert state transitions from any node in that room. Please note if the node belongs to 
another room which isn't silenced it can trigger alert notifications to the users with membership to that other room.
* **Node:** silencing a specific node can be done for the entire space, selecting `All Rooms`, or for specific war room(s). The main difference is
if the node should be silenced for the entire space or just for specific rooms (when specific rooms are selected only users with membership to that room won't receive notifications).

### Scope definition for Alerts
* **Alert name:** silencing a specific alert name silences all alert state transitions for that specific alert. 
* **Alert context:** silencing a specific alert context will silence all alert state transitions for alerts targeting that chart context, for more details check [alert configuration docs](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md#alarm-line-on).
* **Alert role:** silencing a specific alert role will silence all the alert state transitions for alerts that are configured to be specific role recipients, for more details check [alert configuration docs](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md#alarm-line-to).

Beside the above two main entities there are another two important settings that you can define on a silencing rule:
* Who does the rule affect? **All user** in the space or **Myself**
* When does is to apply? **Immediately** or on a **Schedule** (when setting immediately you can set duration)

For further help on setting alert notification silencing rules go to [Manage Alert Notification Silencing Rules](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/manage-alert-notification-silencing-rules.md).

> ⚠️ This feature is only available for [Netdata paid plans](https://github.com/netdata/netdata/edit/master/docs/cloud/manage/plans.md).

## Flood protection

If a node has too many state changes like firing too many alerts or going from reachable to unreachable, Netdata Cloud
enables flood protection. As long as a node is in flood protection mode, Netdata Cloud does not send notifications about
this node. Even with flood protection active, it is possible to access the node directly, either via Netdata Cloud or
the local Agent dashboard at `http://NODE:19999`.

## Anatomy of an alert notification

Email alarm notifications show the following information:

- The Space's name
- The node's name
- Alarm status: critical, warning, cleared
- Previous alarm status
- Time at which the alarm triggered
- Chart context that triggered the alarm
- Name and information about the triggered alarm
- Alarm value
- Total number of warning and critical alerts on that node
- Threshold for triggering the given alarm state
- Calculation or database lookups that Netdata uses to compute the value
- Source of the alarm, including which file you can edit to configure this alarm on an individual node

Email notifications also feature a **Go to Node** button, which takes you directly to the offending chart for that node
within Cloud's embedded dashboards.

Here's an example email notification for the `ram_available` chart, which is in a critical state:

![Screenshot of an alarm notification email from Netdata Cloud](https://user-images.githubusercontent.com/1153921/87461878-e933c480-c5c3-11ea-870b-affdb0801854.png)
