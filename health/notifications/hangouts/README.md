# Hangouts Chat

This is what you will get:
![Netdata on Hangouts](https://imgur.com/a/FVlZ3F0)
You need:

1. The **incoming webhook URL** as given by Hangouts Chat. You can use the same on all your netdata servers (or you can have multiple if you like - your decision).
2. One or more channels to post the messages to.

Get them here: https://developers.google.com/hangouts/chat/how-tos/webhooks

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
#------------------------------------------------------------------------------
# hangouts (hangouts chat) global notification options

# multiple recipients can be given like this:
#                  "CHANNEL1 CHANNEL2 ..."

# enable/disable sending Hangouts notifications
SEND_HANGOUTS="YES"

# Login to Hangouts Chat and create an incoming webhook. You need only one for all
# your netdata servers (or you can have one for each of your netdata).
# Without it, netdata cannot send Hangouts notifications.
HANGOUTS_WEBHOOK_URL="<your_incoming_webhook_url>"

# if a role's recipients are not configured, a notification will be send to
# this Hangouts channel (empty = do not send a notification for unconfigured
# roles).
DEFAULT_RECIPIENT_HANGOUTS="monitoring_alarms"

```

You can define multiple channels like this: `alarms systems`.
You can give different channels per **role** using these (at the same file):

```
role_recipients_hangouts[sysadmin]="systems"
role_recipients_hangouts[dba]="databases systems"
role_recipients_hangouts[webmaster]="marketing development"
```

The keywords `systems`, `databases`, `marketing`, `development` are Hangouts channels (they should already exist).
Both public and private channels can be used, even if they differ from the channel configured in yout Hangouts incomming webhook.

