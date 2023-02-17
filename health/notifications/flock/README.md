<!--
title: "Flock agent alert notifications"
sidebar_label: "Flock"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/flock/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Flock agent alert notifications

This is what you will get:

![Flock](https://i.imgur.com/ok9bRzw.png)

You need:

The **incoming webhook URL** as given by flock.com. 
You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).

Get them here: <https://admin.flock.com/webhooks>

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# sending flock notifications

# enable/disable sending pushover notifications
SEND_FLOCK="YES"

# Login to flock.com and create an incoming webhook.
# You need only one for all your Netdata servers.
# Without it, Netdata cannot send flock notifications.
FLOCK_WEBHOOK_URL="https://api.flock.com/hooks/sendMessage/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# if a role recipient is not configured, no notification will be sent
DEFAULT_RECIPIENT_FLOCK="alarms"
```


