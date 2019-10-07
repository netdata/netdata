# Hangouts Chat

This is what you will get:
![Netdata on Hangouts](https://imgur.com/a/FVlZ3F0)
You need:

1. A room or more rooms
2. In each room create a **Incoming Webhooks**

How to create a incoming webhooks: 
https://developers.google.com/hangouts/chat/how-tos/webhooks

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

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
# if a DEFAULT_RECIPIENT_HANGOUTS are not configured,
# notifications wouldn't be send to hangouts rooms.
# DEFAULT_RECIPIENT_HANGOUTS="systems development|critical"
DEFAULT_RECIPIENT_HANGOUTS="sysadmin devops alarms|critical"
```
You can define multiple rooms like this: `sysadmin devops alarms|critical`.

The keywords `sysadmin`, `devops` and `alarms` are Hangouts rooms.
