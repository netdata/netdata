<!--
---
title: "Discordapp.com"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/discord/README.md
---
-->

# Discordapp.com

This is what you will get:

![image](https://cloud.githubusercontent.com/assets/7321975/22215935/b49ede7e-e162-11e6-98d0-ae8541e6b92e.png)

You need:

1.  The **incoming webhook URL** as given by Discord. Create a webhook by following the official [Discord documentation](https://support.discordapp.com/hc/en-us/articles/228383668-Intro-to-Webhooks). You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
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
# https://support.discordapp.com/hc/en-us/articles/228383668-Intro-to-Webhooks
DISCORD_WEBHOOK_URL="https://discordapp.com/api/webhooks/XXXXXXXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

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

The keywords `systems`, `databases`, `marketing`, `development` are discordapp.com channels (they should already exist within your discord server).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fdiscord%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
