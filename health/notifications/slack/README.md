<!--
title: "Slack agent alert notifications"
sidebar_label: "Slack"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/slack/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Slack agent alert notifications

This is what you will get:
![image](https://cloud.githubusercontent.com/assets/2662304/18407116/bbd0fee6-7710-11e6-81cf-58c0defaee2b.png)

You need:

1.  The **incoming webhook URL** as given by slack.com. You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
2.  One or more channels to post the messages to.

To get a webhook that works on multiple channels, you will need to login to your slack.com workspace and create an incoming webhook using the [Incoming Webhooks App](https://slack.com/apps/A0F7XDUAZ-incoming-webhooks).
Do NOT use the instructions in <https://api.slack.com/incoming-webhooks#enable_webhooks>, as the particular webhooks work only for a single channel.

Set the webhook and the recipients in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
SEND_SLACK="YES"

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

-   The recipient defined in slack for the webhook (not known to Netdata)
-   The channel 'alarms'
-   The channel 'systems'
-   The user @myuser

You can give different recipients per **role** using these (at the same file):

```
role_recipients_slack[sysadmin]="systems"
role_recipients_slack[dba]="databases systems"
role_recipients_slack[webmaster]="marketing development"
```


