<!--
title: "Configure alerting notification methods"
sidebar_label: "Configure alerting notification methods"
custom_edit_url: "https://github.com/netdata/netdata/blob/master/docs/tasks/alerting/configure-alerting-notification-methods.md"
learn_status: "Published"
sidebar_position: 4
learn_topic_type: "Tasks"
learn_rel_path: "alerting"
learn_docs_purpose: "Instructions on how to configure alerting notification methods"
-->

In all alert configuration files, the `exec` line defines an external script that will be called once
the alert is triggered. The default script is `alarm-notify.sh`.

:::info
You can change the default script globally by
editing [Netdata's Configuration](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
.
:::

`alarm-notify.sh` is capable of sending notifications:

- to multiple recipients
- using multiple notification methods
- filtering severity per recipient

It uses **roles**. For example `sysadmin`, `webmaster`, `dba`, etc.

Each alert is assigned to one or more roles, using the `to` line of the alert configuration.  
The `alarm-notify.sh` then uses its own configuration file `/etc/netdata/health_alarm_notify.conf`.

Each role may have one or more destinations.

## Prerequisites

- A node with the Agent installed, and terminal access to that node

## Steps

So, to configure an alerting notification method:

1. Edit `/etc/netdata/health_alarm_notify.conf`, if you don't know how to edit configuration files, refer to
   the [Configuring the Agent](https://github.com/netdata/netdata/blob/master/docs/tasks/general-configuration/configure-the-agent.md)
   Task.
2. Proceed on finding the section of the notification method you want to edit, and fill in the required info.
   Specifically, all notification methods except email, require some configuration
   (i.e. API keys, tokens, destination rooms, channels, etc).
3. Or if you want to configure recipients per role for every notification role, you can edit the entries at the bottom
   of the file, and to see the entries for a role (for example `sysadmin`) you can run:

    ```bash
    grep sysadmin /etc/netdata/health_alarm_notify.conf
    ```
   Which will return:
    ```
    role_recipients_email[sysadmin]="${DEFAULT_RECIPIENT_EMAIL}"
    role_recipients_pushover[sysadmin]="${DEFAULT_RECIPIENT_PUSHOVER}"
    role_recipients_pushbullet[sysadmin]="${DEFAULT_RECIPIENT_PUSHBULLET}"
    role_recipients_telegram[sysadmin]="${DEFAULT_RECIPIENT_TELEGRAM}"
    role_recipients_slack[sysadmin]="${DEFAULT_RECIPIENT_SLACK}"
    ...
    ```

## Testing Notifications

You can run the following command by hand, to test alerts configuration:

1. Become the netdata user
    ```bash
    su -s /bin/bash netdata
    ```
2. Enable debugging info on the console
    ```bash
    export NETDATA_ALARM_NOTIFY_DEBUG=1    
    ```
3. Send test alerts to sysadmin
    ```bash
    /usr/libexec/netdata/plugins.d/alarm-notify.sh test
    ```
4. Send test alerts to any role
    ```bash
    /usr/libexec/netdata/plugins.d/alarm-notify.sh test "ROLE"
    ```

Then look in the output for the alarm-notify.sh calls and run the one you want to trace with `bash -x`.
