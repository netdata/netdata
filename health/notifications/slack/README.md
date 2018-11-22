# Slack

This is what you will get:
![image](https://cloud.githubusercontent.com/assets/2662304/18407116/bbd0fee6-7710-11e6-81cf-58c0defaee2b.png)

You need:

1. The **incoming webhook URL** as given by slack.com. You can use the same on all your netdata servers (or you can have multiple if you like - your decision).
2. One or more channels to post the messages to.

Get them here: https://api.slack.com/incoming-webhooks

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# sending slack notifications

# note: multiple recipients can be given like this:
#                  "CHANNEL1 CHANNEL2 ..."

# enable/disable sending pushover notifications
SEND_SLACK="YES"

# Login to slack.com and create an incoming webhook.
# You need only one for all your netdata servers.
# Without it, netdata cannot send slack notifications.
SLACK_WEBHOOK_URL="https://hooks.slack.com/services/XXXXXXXX/XXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# if a role recipient is not configured, a notification will be send to
# this slack channel:
DEFAULT_RECIPIENT_SLACK="alarms"

```

You can define multiple channels like this: `alarms systems`.
You can give different channels per **role** using these (at the same file):

```
role_recipients_slack[sysadmin]="systems"
role_recipients_slack[dba]="databases systems"
role_recipients_slack[webmaster]="marketing development"
```

The keywords `systems`, `databases`, `marketing`, `development` are slack.com channels (they should already exist in slack).
