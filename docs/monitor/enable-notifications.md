<!--
title: "Enable alarm notifications"
description: "Send Netdata alarms from a centralized place with Netdata Cloud, or configure nodes individually, to enable incident response and faster resolution."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/monitor/enable-notifications.md
-->

# Enable alarm notifications

Netdata offers two ways to receive alarm notifications on external platforms. These methods work independently _or_ in
parallel, which means you can enable both at the same time to send alarm notifications to any number of endpoints.

Both methods use a node's health alarms to generate the content of alarm notifications. Read the doc on [configuring
alarms](/docs/monitor/configure-alarms.md) to change the preconfigured thresholds or to create tailored alarms for your
infrastructure.

Netdata Cloud offers [centralized alarm notifications](#netdata-cloud) via email, which leverages the health status
information already streamed to Netdata Cloud from connected nodes to send notifications to those who have enabled them.

The Netdata Agent has a [notification system](#netdata-agent) that supports more than a dozen services, such as email,
Slack, PagerDuty, Twilio, Amazon SNS, Discord, and much more.

For example, use centralized alarm notifications in Netdata Cloud for immediate, zero-configuration alarm notifications
for your team, then configure individual nodes send notifications to a PagerDuty endpoint for an automated incident
response process.

## Netdata Cloud

Netdata Cloud's [centralized alarm
notifications](https://learn.netdata.cloud/docs/cloud/alerts-notifications/notifications) is a zero-configuration way to
get notified when an anomaly or incident strikes any node or application in your infrastructure. The advantage of using
centralized alarm notifications from Netdata Cloud is that you don't have to worry about configuring each node in your
infrastructure.

To enable centralized alarm notifications for a Space, click on **Manage Space** in the left-hand menu, then click on
the **Notifications** tab. Click the toggle switch next to **E-mail** to enable this notification method.

Next, enable notifications on a user level by clicking on your profile icon, then **Profile** in the dropdown. The
**Notifications** tab reveals rich management settings, including the ability to enable/disable methods entirely or
choose what types of notifications to receive from each War Room.

![Enabling and configuring alarm notifications in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/101936280-93c50900-3b9d-11eb-9ba0-d6927fa872b7.gif)

See the [centralized alarm notifications](https://learn.netdata.cloud/docs/cloud/alerts-notifications/notifications)
reference doc for further details about what information is conveyed in an email notification, flood protection, and
more.

## Netdata Agent

The Netdata Agent's [notification system](/health/notifications/README.md) runs on every node and dispatches
notifications based on configured endpoints and roles. You can enable multiple endpoints on any one node _and_ use Agent
notifications in parallel with centralized alarm notifications in Netdata Cloud.

> ❗ If you want to enable notifications from multiple nodes in your infrastructure, each running the Netdata Agent, you
> must configure each node individually.

Below, we'll use [Slack notifications](#enable-slack-notifications) as an example of the process of enabling any
notification platform.

### Supported notification endpoints

-   [**alerta.io**](/health/notifications/alerta/README.md)
-   [**Amazon SNS**](/health/notifications/awssns/README.md)
-   [**Custom endpoint**](/health/notifications/custom/README.md)
-   [**Discord**](/health/notifications/discord/README.md)
-   [**Dynatrace**](/health/notifications/dynatrace/README.md)
-   [**Email**](/health/notifications/email/README.md)
-   [**Flock**](/health/notifications/flock/README.md)
-   [**Google Hangouts**](/health/notifications/hangouts/README.md)
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

### Enable Slack notifications

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

## What's next?

Now that you have health entities configured to your infrastructure's needs and notifications to inform you of anomalies
or incidents, your health monitoring setup is complete.

To make your dashboards most useful during root cause analysis, use Netdata's [distributed data
architecture](/docs/store/distributed-data-architecture.md) for the best-in-class performance and scalability.

### Related reference documentation

- [Netdata Cloud · Alarm notifications](https://learn.netdata.cloud/docs/cloud/alerts-notifications/notifications)
- [Netdata Agent · Notifications](/health/notifications/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fmonitor%2Fenable-notifications&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
