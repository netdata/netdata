<!--
title: "Enable notifications"
description: "Send Netdata's alerts to platforms like email, Slack, PagerDuty, Twilio, and more to enable incident response and faster resolution."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/monitor/enable-notifications.md
-->

# Enable notifications

Netdata comes with a notification system that supports more than a dozen services, such as email, Slack, PagerDuty,
Twilio, Amazon SNS, Discord, and much more. You can enable as many platforms as you want, and configure them to match
your organization's needs with features like role-based notifications.

To see all the supported platforms, visit our [notifications](/health/notifications/README.md) doc.

This doc covers enabling email and Slack notifications, but the same process applies to enabling any other notification
platform.

## Enable email notifications

To use email notifications, you need [`sendmail`](http://www.postfix.org/sendmail.1.html) or an equivalent installed on
your system.

Edit the `health_alarm_notify.conf` file, which resides in your Netdata [config
directory](/docs/configure/nodes.md#netdata-config-directory).

```bash
sudo ./edit-config health_alarm_notify.conf
```

Look for the following lines:

```conf
# if a role recipient is not configured, an email will be sent to:
DEFAULT_RECIPIENT_EMAIL="root"
# to receive only critical alarms, set it to "root|critical"
```

Change the value of `DEFAULT_RECIPIENT_EMAIL` to the email address at which you'd like to receive notifications.

```conf
# if a role recipient is not configured, an email will be sent to:
DEFAULT_RECIPIENT_EMAIL="me@example.com"
# to receive only critical alarms, set it to "root|critical"
```

Test email notifications system by first becoming the Netdata user and then asking Netdata to send a test alarm:

```bash
sudo su -s /bin/bash netdata
/usr/libexec/netdata/plugins.d/alarm-notify.sh test
```

You should see output similar to this:

```bash
# SENDING TEST WARNING ALARM TO ROLE: sysadmin
2019-10-17 18:23:38: alarm-notify.sh: INFO: sent email notification for: hostname test.chart.test_alarm is WARNING to 'me@example.com'
# OK

# SENDING TEST CRITICAL ALARM TO ROLE: sysadmin
2019-10-17 18:23:38: alarm-notify.sh: INFO: sent email notification for: hostname test.chart.test_alarm is CRITICAL to 'me@example.com'
# OK

# SENDING TEST CLEAR ALARM TO ROLE: sysadmin
2019-10-17 18:23:39: alarm-notify.sh: INFO: sent email notification for: hostname test.chart.test_alarm is CLEAR to 'me@example.com'
# OK
```

Check your email. You should receive three separate emails for each health status change: `WARNING`, `CRITICAL`, and
`CLEAR`.

See the [email notifications](/health/notifications/email/README.md) doc for more options and information.

## Enable Slack notifications

If you're one of the many who spend their workday getting pinged with GIFs by your colleagues, why not add Netdata
notifications to the mix? It's a great way to immediately see, collaborate around, and respond to anomalies in your
infrastructure.

To get Slack notifications working, you first need to add an [incoming
webhook](https://slack.com/apps/A0F7XDUAZ-incoming-webhooks) to the channel of your choice. Click the green **Add to
Slack** button, choose the channel, and click the **Add Incoming WebHooks Integration** button.

On the following page, you'll receive a **Webhook URL**. That's what you'll need to configure Netdata, so keep it handy.

Time to dive back into your `health_alarm_notify.conf` file:

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

Time to test the notifications again:

```bash
sudo su -s /bin/bash netdata
/usr/libexec/netdata/plugins.d/alarm-notify.sh test
```

You should receive three notifications in your Slack channel for each health status change: `WARNING`, `CRITICAL`, and
`CLEAR`.

See the [Slack notifications](/health/notifications/slack/README.md) doc for more options and information.

## What's next?

Learn more about Netdata's notifications system in the [notifications](/health/notifications/README.md) docs.

Now that you have health entities configured to your infrastructure's needs, and notifications to inform you of
anomalies, you have everything you need to monitor the health of your infrastructure. To make your dashboards most
useful during root cause analysis, you can use Netdata's [distributed data
architecture](/docs/store/distributed-data-architecture.md) for the best-in-class performance and scalability.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fmonitor%2Fenable-notifications&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
