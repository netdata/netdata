
<!--
Title: "Notifications"
custom_edit_url: https://github.com/netdata/netdata/blob/master/docs/concepts/health-monitoring/notifications.md
learn_status: Published
learn_topic_type: Concepts
learn_rel_path: docs/concepts/health-monitoring/notifications.md

learn_docs_purpose: Explain the notifications mechanism, methods to receive notification and give a visual overview of the integrations 
-->



**********************************************************************
Template:

Small intro, what we are about to cover

// every concept we will explain to this document (grouped) should be a different heading (h2) and followed by an example
// we need at any given moment to provide a reference (a anchored link to this concept)
## concept title

A concept introduces a single feature or concept. A concept should answer the questions:

1. What is this?
2. Why would I use it?

For instance, for example etc etc

Give a small taste for this concept, not trying to cover it's reference page. 

In the end of the document:

## Related topics

list of related topics

*****************Suggested document to be transformed**************************From netdata repo's commit : 3a672f5b4ba23d455b507c8276b36403e10f953d<!--
title: "Alarm notifications"
description: "Reference documentation for Netdata's alarm notification feature, which supports dozens of endpoints, user roles, and more."
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/README.md
-->

# Alarm notifications

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

So, for example the `sysadmin` role may send:

1.  emails to admin1@example.com and admin2@example.com
2.  pushover.net notifications to USERTOKENS `A`, `B` and `C`.
3.  pushbullet.com push notifications to admin1@example.com and admin2@example.com
4.  messages to slack.com channel `#alarms` and `#systems`.
5.  messages to Discord channels `#alarms` and `#systems`.

## Configuration

Edit `/etc/netdata/health_alarm_notify.conf` by running `/etc/netdata/edit-config health_alarm_notify.conf`:

-   settings per notification method:

     all notification methods except email, require some configuration
     (i.e. API keys, tokens, destination rooms, channels, etc).

-  **recipients** per **role** per **notification method**

```sh
grep sysadmin /etc/netdata/health_alarm_notify.conf

role_recipients_email[sysadmin]="${DEFAULT_RECIPIENT_EMAIL}"
role_recipients_pushover[sysadmin]="${DEFAULT_RECIPIENT_PUSHOVER}"
role_recipients_pushbullet[sysadmin]="${DEFAULT_RECIPIENT_PUSHBULLET}"
role_recipients_telegram[sysadmin]="${DEFAULT_RECIPIENT_TELEGRAM}"
role_recipients_slack[sysadmin]="${DEFAULT_RECIPIENT_SLACK}"
...
```

## Testing Notifications

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
