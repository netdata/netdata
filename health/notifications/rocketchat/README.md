# Rocket.Chat notifications

This is what you will get:
![Netdata on RocketChat](https://i.imgur.com/Zu4t3j3.png)
You need:

1. The **incoming webhook URL** as given by RocketChat. You can use the same on all your netdata servers (or you can have multiple if you like - your decision).
2. One or more channels to post the messages to.

Get them here: https://rocket.chat/docs/administrator-guides/integrations/index.html#how-to-create-a-new-incoming-webhook

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
#------------------------------------------------------------------------------
# rocketchat (rocket.chat) global notification options

# multiple recipients can be given like this:
#                  "CHANNEL1 CHANNEL2 ..."

# enable/disable sending rocketchat notifications
SEND_ROCKETCHAT="YES"

# Login to rocket.chat and create an incoming webhook. You need only one for all
# your netdata servers (or you can have one for each of your netdata).
# Without it, netdata cannot send rocketchat notifications.
ROCKETCHAT_WEBHOOK_URL="<your_incoming_webhook_url>"

# if a role's recipients are not configured, a notification will be send to
# this rocketchat channel (empty = do not send a notification for unconfigured
# roles).
DEFAULT_RECIPIENT_ROCKETCHAT="monitoring_alarms"

```

You can define multiple channels like this: `alarms systems`.
You can give different channels per **role** using these (at the same file):

```
role_recipients_rocketchat[sysadmin]="systems"
role_recipients_rocketchat[dba]="databases systems"
role_recipients_rocketchat[webmaster]="marketing development"
```

The keywords `systems`, `databases`, `marketing`, `development` are RocketChat channels (they should already exist).
Both public and private channels can be used, even if they differ from the channel configured in yout RocketChat incomming webhook.
