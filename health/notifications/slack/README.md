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
#                  "RECIPIENT1 RECIPIENT2 ..."

# enable/disable sending pushover notifications
SEND_SLACK="YES"

# Login to slack.com and create an incoming webhook.
# You need only one for all your netdata servers.
# Without it, netdata cannot send slack notifications.
SLACK_WEBHOOK_URL="https://hooks.slack.com/services/XXXXXXXX/XXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# if a role's recipients are not configured, a notification will be send to:
# - A slack channel (syntax: '#channel' or 'channel')  
# - A slack user (syntax: '@user')
# - The channel or user defined in slack for the webhook (syntax: '#')
# empty = do not send a notification for unconfigured roles 
DEFAULT_RECIPIENT_SLACK="alarms"

```

You can define multiple recipients like this: `# #alarms systems @myuser`. 
This example will send the alarm to:
- The recipient defined in slack for the webhook (not known to netdata)
- The channel 'alarms'
- The channel 'systems'
- The user @myuser

You can give different recipients per **role** using these (at the same file):

```
role_recipients_slack[sysadmin]="systems"
role_recipients_slack[dba]="databases systems"
role_recipients_slack[webmaster]="marketing development"
```
