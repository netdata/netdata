<!--
title: "Microsoft Teams"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/msteams/README.md
-->

# Microsoft Teams

This is what you will get:
![image](https://user-images.githubusercontent.com/1122372/92710359-0385e680-f358-11ea-8c52-f366a4fb57dd.png)

You need:

1.  The **incoming webhook URL** as given by Microsoft Teams. You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
2.  One or more channels to post the messages to.

In Microsoft Teams the channel name is encoded in the URI after `/IncomingWebhook/` (for clarity the marked with `[]` in the following example): `https://outlook.office.com/webhook/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX@XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX/IncomingWebhook/[XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX]/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX`

You have to replace the encoded channel name by the placeholder `CHANNEL` in `MSTEAMS_WEBHOOK_URL`. The placeholder `CHANNEL` will be replaced by the actual encoded channel name before sending the notification. This makes it possible to publish to several channels in the same team.

The encoded channel name must then be added to `DEFAULT_RECIPIENTS_MSTEAMS` or to one of the specific variables `role_recipients_msteams[]`. **At least one channel is mandatory for `DEFAULT_RECIPIENTS_MSTEAMS`.**

Set the webhook and the recipients in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
SEND_MSTEAMS="YES"

MSTEAMS_WEBHOOK_URL="https://outlook.office.com/webhook/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX@XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX/IncomingWebhook/CHANNEL/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"

DEFAULT_RECIPIENT_MSTEAMS="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

You can define multiple recipients by listing the encoded channel names like this: `XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY`. 
This example will send the alarm to the two channels specified by their encoded channel names.

You can give different recipients per **role** using these (in the same file):

```
role_recipients_msteams[sysadmin]="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
role_recipients_msteams[dba]="YYYYYYYYYYYYYYYYYYYYYYYYYYYYYYYY"
role_recipients_msteams[webmaster]="ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"
```


