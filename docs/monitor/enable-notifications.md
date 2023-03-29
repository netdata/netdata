<!--
title: "Alert notifications"
description: "Send Netdata alarms from a centralized place with Netdata Cloud, or configure nodes individually, to enable incident response and faster resolution."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/monitor/enable-notifications.md"
sidebar_label: "Notify"
learn_status: "Published"
learn_rel_path: "Integrations/Notify"
-->

# Alert notifications

Netdata offers two ways to receive alert notifications on external platforms. These methods work independently _or_ in
parallel, which means you can enable both at the same time to send alert notifications to any number of endpoints.

Both methods use a node's health alerts to generate the content of alert notifications. Read our documentation on [configuring alerts](https://github.com/netdata/netdata/blob/master/health/REFERENCE.md) to change the preconfigured thresholds or to create tailored alerts for your
infrastructure.

Netdata Cloud offers [centralized alert notifications](#netdata-cloud) via email, which leverages the health status
information already streamed to Netdata Cloud from connected nodes to send notifications to those who have enabled them.

The Netdata Agent has a [notification system](#netdata-agent) that supports more than a dozen services, such as email,
Slack, PagerDuty, Twilio, Amazon SNS, Discord, and much more.

For example, use centralized alert notifications in Netdata Cloud for immediate, zero-configuration alert notifications
for your team, then configure individual nodes send notifications to a PagerDuty endpoint for an automated incident
response process.

## Netdata Cloud

Netdata Cloud's [centralized alert
notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md) is a zero-configuration way to
get notified when an anomaly or incident strikes any node or application in your infrastructure. The advantage of using
centralized alert notifications from Netdata Cloud is that you don't have to worry about configuring each node in your
infrastructure.

To enable centralized alert notifications for a Space, click on **Manage Space** in the left-hand menu, then click on
the **Notifications** tab. Click the toggle switch next to **E-mail** to enable this notification method.

Next, enable notifications on a user level by clicking on your profile icon, then **Profile** in the dropdown. The
**Notifications** tab reveals rich management settings, including the ability to enable/disable methods entirely or
choose what types of notifications to receive from each War Room.

![Enabling and configuring alert notifications in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/101936280-93c50900-3b9d-11eb-9ba0-d6927fa872b7.gif)

See the [centralized alert notifications](https://github.com/netdata/netdata/blob/master/docs/cloud/alerts-notifications/notifications.md)
reference doc for further details about what information is conveyed in an email notification, flood protection, and
more.

## Netdata Agent

The Netdata Agent's [notification system](https://github.com/netdata/netdata/blob/master/health/notifications/README.md) runs on every node and dispatches
notifications based on configured endpoints and roles. You can enable multiple endpoints on any one node _and_ use Agent
notifications in parallel with centralized alert notifications in Netdata Cloud.

> ❗ If you want to enable notifications from multiple nodes in your infrastructure, each running the Netdata Agent, you
> must configure each node individually.

Below, we'll use [Slack notifications](#enable-slack-notifications) as an example of the process of enabling any
notification platform.

### Supported notification endpoints

-   [**alerta.io**](https://github.com/netdata/netdata/blob/master/health/notifications/alerta/README.md)
-   [**Amazon SNS**](https://github.com/netdata/netdata/blob/master/health/notifications/awssns/README.md)
-   [**Custom endpoint**](https://github.com/netdata/netdata/blob/master/health/notifications/custom/README.md)
-   [**Discord**](https://github.com/netdata/netdata/blob/master/health/notifications/discord/README.md)
-   [**Dynatrace**](https://github.com/netdata/netdata/blob/master/health/notifications/dynatrace/README.md)
-   [**Email**](https://github.com/netdata/netdata/blob/master/health/notifications/email/README.md)
-   [**Flock**](https://github.com/netdata/netdata/blob/master/health/notifications/flock/README.md)
-   [**Gotify**](https://github.com/netdata/netdata/blob/master/health/notifications/gotify/README.md)
-   [**IRC**](https://github.com/netdata/netdata/blob/master/health/notifications/irc/README.md)
-   [**Kavenegar**](https://github.com/netdata/netdata/blob/master/health/notifications/kavenegar/README.md)
-   [**Matrix**](https://github.com/netdata/netdata/blob/master/health/notifications/matrix/README.md)
-   [**Messagebird**](https://github.com/netdata/netdata/blob/master/health/notifications/messagebird/README.md)
-   [**Microsoft Teams**](https://github.com/netdata/netdata/blob/master/health/notifications/msteams/README.md)
-   [**Netdata Agent dashboard**](https://github.com/netdata/netdata/blob/master/health/notifications/web/README.md)
-   [**Opsgenie**](https://github.com/netdata/netdata/blob/master/health/notifications/opsgenie/README.md)
-   [**PagerDuty**](https://github.com/netdata/netdata/blob/master/health/notifications/pagerduty/README.md)
-   [**Prowl**](https://github.com/netdata/netdata/blob/master/health/notifications/prowl/README.md)
-   [**PushBullet**](https://github.com/netdata/netdata/blob/master/health/notifications/pushbullet/README.md)
-   [**PushOver**](https://github.com/netdata/netdata/blob/master/health/notifications/pushover/README.md)
-   [**Rocket.Chat**](https://github.com/netdata/netdata/blob/master/health/notifications/rocketchat/README.md)
-   [**Slack**](https://github.com/netdata/netdata/blob/master/health/notifications/slack/README.md)
-   [**SMS Server Tools 3**](https://github.com/netdata/netdata/blob/master/health/notifications/smstools3/README.md)
-   [**StackPulse**](https://github.com/netdata/netdata/blob/master/health/notifications/stackpulse/README.md)
-   [**Syslog**](https://github.com/netdata/netdata/blob/master/health/notifications/syslog/README.md)
-   [**Telegram**](https://github.com/netdata/netdata/blob/master/health/notifications/telegram/README.md)
-   [**Twilio**](https://github.com/netdata/netdata/blob/master/health/notifications/twilio/README.md)


