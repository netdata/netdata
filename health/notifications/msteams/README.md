<!--
---
title: "Microsoft Teams"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/msteams/README.md
---
-->

# Microsoft Teams

This is what you will get:
![image](https://user-images.githubusercontent.com/1122372/92710359-0385e680-f358-11ea-8c52-f366a4fb57dd.png)

You need:

1.  The **incoming webhook URL** as given by Microsoft Teams. You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
2.  One or more channels to post the messages to.

TODO
For Microsoft Teams the channel name is encoded in the URI after `/IncomingWebhook/`: (for clarity the marked with `[]` in the following example: `https://outlook.office.com/webhook/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX@XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX/IncomingWebhook/[XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX]/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX`

In order to get it working properly, you have to replace the value between [] ....IncomingWebhook/[___]/..... by "CHANNEL" string for `MSTEAMS_WEBHOOK_URL`.

If a role's recipients are not configured, a notification will be send to this team channel (empty = do not send a notification for unconfigured roles): For Microsoft Teams the channel name is encoded in the URI after ....IncomingWebhook/[___]/.....  This value will be replaced in the webhook value to publish to several channels in a same Team.
AT LEAST ONE CHANNEL IS MANDATORY

Set the webhook and the recipients in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
SEND_MSTEAMS="YES"

MSTEAMS_WEBHOOK_URL="https://outlook.office.com/webhook/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX@XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX/IncomingWebhook/CHANNEL/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"

DEFAULT_RECIPIENT_MSTEAMS="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

You can define multiple recipients by listing the encoded channel names like this: `XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY`. 
This example will send the alarm to the two channels specified by their encoded channel names.

You can give different recipients per **role** using these (at the same file):

```
role_recipients_msteam[sysadmin]="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
role_recipients_msteam[dba]="YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY"
role_recipients_msteam[webmaster]="ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fmsteams%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
