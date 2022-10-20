<!--
title: "notifications"
sidebar_label: "Notifications"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/notifications.md"
sidebar_position: 3
learn_status: "Published"
learn_topic_type: "Concepts"
learn_rel_path: "health-monitoring"
learn_docs_purpose: "Define notifications, correlate how they can assist with health monitoring. Give an explanation of notification methods."
-->

**********************************************************************

Netdata can send centralized alert notifications to your team whenever a node enters a warning, critical, or
unreachable state. By enabling notifications, you ensure no alert, on any node in your infrastructure, goes unnoticed by
you or your team.

If a node is getting disconnected often or has many alerts, we protect you and your team from alert fatigue by sending
you a flood protection notification. Getting one of these notifications is a good signal of health or performance issues
on that node.

Netdata Cloud currently supports email notifications. We're working on additional endpoints and functionality.

Admins must enable alert notifications for their [Space(s)](#manage-alert-notifications-for-a-space). All users in a
Space can then personalize their notifications settings from within their [account
menu](#manage-alert-notifications-per-user).

:::note

Centralized alert notifications from Netdata Cloud is a independent process from notifications from
Netdata. You can enable one or the other, or both, based on your needs. However,
the alerts you see in Netdata Cloud are based on those streamed from your Netdata-monitoring nodes. If you want to tweak
or add new alert that you see in Netdata Cloud, and receive via centralized alert notifications, you must
configure each node's alert watchdog.

:::

## Alert notifications for a Cloud Space

To enable notifications for a Space, click **Manage Space** in the [Space](/docs/cloud/spaces) management area.

![The Manage Space
button](https://user-images.githubusercontent.com/1153921/108530321-b6a52500-7292-11eb-9599-a5ba77a25094.png)

In the modal, click on the **Notifications** tab. This menu option is visible only to administrators.

![The Space-level management panel for alert
notifications](https://user-images.githubusercontent.com/1153921/99722010-3f7eab80-2a6d-11eb-8836-547a6d243d51.png)

Click on the toggle to enable or disable a notification method.

## Alert notifications per Cloud user

You, and other individual users in your Space, can also enable and disable notification methods.

Click on your profile icon at the top-right of the Cloud UI to open your account menu, then **Profile** in the dropdown. 
Click on the **Notifications** tab in the panel that appears.

![The user-level management panel for alert
notifications](https://user-images.githubusercontent.com/1153921/99722015-40174200-2a6d-11eb-837e-3de761127ca7.png)

Enable or disable notification methods with the toggle buttons.

Select which the notifications you want to receive from each War Room:

-   **All alerts and unreachable**: Receive notifications for all changes in alert status: critical, warning, and
    cleared. In addition, receive notifications for any node that enters an unreachable state.
-   **All alerts**: Receive notifications for all changes in alert status: critical, warning, and cleared.
-   **Critical only**: Receive notifications only for critical alerts.
-   **No notifications**: Receive no notifications for nodes in this War Room.

If a Space's administrator has disabled notifications, you will see a mesage similar to, "E-mail notifications for this space has been disabled by admin," and your settings have no effect.

## Flood protection

If a node has too many state changes like firing too many alerts or going from reachable to unreachable, Netdata Cloud
enables flood protection. As long as a node is in flood protection mode, Netdata Cloud does not send notifications about
this node. Even with flood protection active, it is possible to access the node directly, either via Netdata Cloud or
the local Agent dashboard at `http://NODE:19999`.

## Anatomy of an alert notification

Email alert notifications show the following information:

-   The Space's name
-   The node's name
-   Alarm status: critical, warning, cleared
-   Previous alarm status
-   Time at which the alarm triggered
-   Chart context that triggered the alarm
-   Name and information about the triggered alarm
-   Alarm value
-   Total number of warning and critical alerts on that node
-   Threshold for triggering the given alarm state
-   Calculation or database lookups that Netdata uses to compute the value
-   Source of the alarm, including which file you can edit to configure this alarm on an individual node

Email notifications also feature a **Go to Node** button, which takes you directly to the offending chart for that node
within Cloud's embedded dashboards.

Here's an example email notification for the `ram_available` chart, which is in a critical state:

![Screenshot of an alarm notification email from Netdata
Cloud](https://user-images.githubusercontent.com/1153921/87461878-e933c480-c5c3-11ea-870b-affdb0801854.png)

The `exec` line in health configuration defines an external script that will be called once
the alarm is triggered. The default script is `alarm-notify.sh`.

You can change the default script globally by editing `/etc/netdata/netdata.conf`.

`alarm-notify.sh` is capable of sending notifications:

-   to multiple recipients
-   using multiple notification methods
-   filtering severity per recipient

It uses **roles**. For example `sysadmin`, `webmaster`, `dba`, etc.

Each alarm is assigned to one or more roles, using the `to` line of the alarm configuration. Then `alarm-notify.sh` uses
its own configuration file `/etc/netdata/health_alarm_notify.conf`. To edit it on your system, run
`/etc/netdata/edit-config health_alarm_notify.conf` and find the destination address of the notification for each
method.

Each role may have one or more destinations.

So, for example, the `sysadmin` role may send:

1.  emails to admin1@example.com and admin2@example.com
2.  pushover.net notifications to USERTOKENS `A`, `B` and `C`.
3.  pushbullet.com push notifications to admin1@example.com and admin2@example.com
4.  messages to slack.com channel `#alarms` and `#systems`.
5.  messages to Discord channels `#alarms` and `#systems`.

## Netdata Agent alert notifications

The Netdata Agent's [notification system](/health/notifications/README.md) runs on every node and dispatches
notifications based on configured endpoints and roles. You can enable multiple endpoints on any one node _and_ use Agent
notifications in parallel with centralized alarm notifications in Netdata Cloud.

> ❗ If you want to enable notifications from multiple nodes in your infrastructure, each running the Netdata Agent, you
> must configure each node individually.

In the next sections, we'll list the notification endpoints that are supported, and then we'll use a [Slack notifications example](#enable-slack-notifications) 
as an illustration of the process of enabling any notification platform.

### Supported notification endpoints

-   [**alerta.io**](/health/notifications/alerta/README.md)
-   [**Amazon SNS**](/health/notifications/awssns/README.md)
-   [**Custom endpoint**](/health/notifications/custom/README.md)
-   [**Discord**](/health/notifications/discord/README.md)
-   [**Dynatrace**](/health/notifications/dynatrace/README.md)
-   [**Email**](/health/notifications/email/README.md)
-   [**Flock**](/health/notifications/flock/README.md)
-   [**Google Hangouts**](/health/notifications/hangouts/README.md)
-   [**Gotify**](/health/notifications/gotify/README.md)
-   [**IRC**](/health/notifications/irc/README.md)
-   [**Kavenegar**](/health/notifications/kavenegar/README.md)
-   [**Matrix**](/health/notifications/matrix/README.md)
-   [**Messagebird**](/health/notifications/messagebird/README.md)
-   [**Microsoft Teams**](/health/notifications/msteams/README.md)
-   [**Netdata Agent dashboard**](/health/notifications/web/README.md)
-   [**Opsgenie**](/health/notifications/opsgenie/README.md)
-   [**PagerDuty**](/health/notifications/pagerduty/README.md)
-   [**Prowl**](/health/notifications/prowl/README.md)
-   [**PushBullet**](/health/notifications/pushbullet/README.md)
-   [**PushOver**](/health/notifications/pushover/README.md)
-   [**Rocket.Chat**](/health/notifications/rocketchat/README.md)
-   [**Slack**](/health/notifications/slack/README.md)
-   [**SMS Server Tools 3**](/health/notifications/smstools3/README.md)
-   [**StackPulse**](/health/notifications/stackpulse/README.md)
-   [**Syslog**](/health/notifications/syslog/README.md)
-   [**Telegram**](/health/notifications/telegram/README.md)
-   [**Twilio**](/health/notifications/twilio/README.md)

### Example: Enable Slack notifications 

First, [Add an incoming webhook](https://slack.com/apps/A0F7XDUAZ-incoming-webhooks) in Slack for the channel where you
want to see alarm notifications from Netdata. Click the green **Add to Slack** button, choose the channel, and click the
**Add Incoming WebHooks Integration** button.

On the following page, you'll receive a **Webhook URL**. That's what you'll need to configure Netdata, so keep it handy.

Navigate to your [Netdata config directory](/docs/configure/nodes.md#the-netdata-config-directory) and use `edit-config` to
open the `health_alarm_notify.conf` file:

```bash
sudo ./edit-config health_alarm_notify.conf
```

Look for the `SLACK_WEBHOOK_URL="  "` line and add the incoming webhook URL you got from Slack:

```conf
SLACK_WEBHOOK_URL="https://hooks.slack.com/services/XXXXXXXXX/XXXXXXXXX/XXXXXXXXXXXX"
```

A few lines down, edit the `DEFAULT_RECIPIENT_SLACK` line to contain a single hash `#` character. This instructs Netdata
to send a notification to the channel you configured with the incoming webhook.

```conf
DEFAULT_RECIPIENT_SLACK="#"
```

To test Slack notifications, switch to the Netdata user.

```bash
sudo su -s /bin/bash netdata
```

Next, run the `alarm-notify` script using the `test` option.

```bash
/usr/libexec/netdata/plugins.d/alarm-notify.sh test
```

You should receive three notifications in your Slack channel for each health status change: `WARNING`, `CRITICAL`, and
`CLEAR`.

See the [Agent Slack notifications](/health/notifications/slack/README.md) doc for more options and information.


## Notification Test

You can run the following command by hand, to test alarms configuration:

```sh
# become user netdata
su -s /bin/bash netdata

# enable debugging info on the console
export NETDATA_ALARM_NOTIFY_DEBUG=1

# send test alarms to sysadmin
/usr/libexec/netdata/plugins.d/alarm-notify.sh test

# send test alarms to any role
/usr/libexec/netdata/plugins.d/alarm-notify.sh test "ROLE"
```

Note that in versions before 1.16, the plugins.d directory may be installed in a different location in certain OSs (e.g. under `/usr/lib/netdata`). You can always find the location of the alarm-notify.sh script in `netdata.conf`.

If you need to dig even deeper, you can trace the execution with `bash -x`. Note that in test mode, alarm-notify.sh calls itself with many more arguments. So first do

```sh
bash -x /usr/libexec/netdata/plugins.d/alarm-notify.sh test
```

 Then look in the output for the alarm-notify.sh calls and run the one you want to trace with `bash -x`. 

*******************************************************************************
