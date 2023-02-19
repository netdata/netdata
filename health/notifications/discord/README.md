<!--
title: "Discord agent alert notifications"
sidebar_label: "Discord"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/health/notifications/discord/README.md"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Discord agent alert notifications

This is what you will get:

![image](https://cloud.githubusercontent.com/assets/7321975/22215935/b49ede7e-e162-11e6-98d0-ae8541e6b92e.png)

You need:

1.  The **incoming webhook URL** as given by Discord. Create a webhook by following the official [Discord documentation](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks). You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
2.  One or more Discord channels to post the messages to.

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# sending discord notifications

# note: multiple recipients can be given like this:
#                  "CHANNEL1 CHANNEL2 ..."

# enable/disable sending discord notifications
SEND_DISCORD="YES"

# Create a webhook by following the official documentation -
# https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks
DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/XXXXXXXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# if a role's recipients are not configured, a notification will be send to
# this discord channel (empty = do not send a notification for unconfigured
# roles):
DEFAULT_RECIPIENT_DISCORD="alarms"
```

You can define multiple channels like this: `alarms systems`.
You can give different channels per **role** using these (at the same file):

```
role_recipients_discord[sysadmin]="systems"
role_recipients_discord[dba]="databases systems"
role_recipients_discord[webmaster]="marketing development"
```

The keywords `systems`, `databases`, `marketing`, `development` are discord.com channels (they should already exist within your discord server).
