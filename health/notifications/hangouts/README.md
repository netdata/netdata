<!--
title: "Google Hangouts agent alert notifications"
description: "Send alerts to Send notifications to Google Hangouts any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "Google Hangouts"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/hangouts/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Google Hangouts agent alert notifications

[Google Hangouts](https://hangouts.google.com/) is a cross-platform messaging app developed by Google. You can configure
Netdata to send alarm notifications to a Hangouts room in order to stay aware of possible health or performance issues
on your nodes. Here's an example of the notification in action:

![Netdata on Hangouts](https://user-images.githubusercontent.com/1153921/66427166-47de6900-e9c8-11e9-8322-b4b03f084dc1.png)

To receive notifications in Google Hangouts, you need the following in your Hangouts setup:

1.   One or more rooms.
2.   An **incoming webhook** for each room.

Follow [Google's documentation](https://developers.google.com/hangouts/chat/how-tos/webhooks) to create an incoming
webhook for each room you want to send Netdata notifications to.

Set the webhook URIs and room names in `health_alarm_notify.conf`. To edit it on your system, run
`/etc/netdata/edit-config health_alarm_notify.conf`):

## Threads (optional)

Instead to receive alarms on different threads, Netdata allows you to concentrate them inside an unique thread when you
set the variable `HANGOUTS_WEBHOOK_THREAD[NAME]`.

```
#------------------------------------------------------------------------------
# hangouts (google hangouts chat) global notification options
# enable/disable sending hangouts notifications
SEND_HANGOUTS="YES"
# On Hangouts, in the room you choose, create an incoming webhook,
# copy the link and paste it below and also identify the room name.
# Without it, netdata cannot send hangouts notifications to that room.
# HANGOUTS_WEBHOOK_URI[ROOM_NAME]="URLforroom1"
HANGOUTS_WEBHOOK_URI[systems]="https://chat.googleapis.com/v1/spaces/AAAAXXXXXXX/..."
HANGOUTS_WEBHOOK_URI[development]="https://chat.googleapis.com/v1/spaces/AAAAYYYYY/..."
# On Hangouts, copy a thread link and change the values for space and thread
# HANGOUTS_WEBHOOK_THREAD[systems]="spaces/AAAAXXXXXXX/threads/XXXXXXXXXXX"
# if a DEFAULT_RECIPIENT_HANGOUTS are not configured,
# notifications wouldn't be send to hangouts rooms.
# DEFAULT_RECIPIENT_HANGOUTS="systems development|critical"
DEFAULT_RECIPIENT_HANGOUTS="sysadmin devops alarms|critical"
```

You can define multiple rooms like this: `sysadmin devops alarms|critical`.

The keywords `sysadmin`, `devops`, and `alarms` are Hangouts rooms.


