# Discord Agent alert notifications

Learn how to send notifications to Discord using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:

![image](https://cloud.githubusercontent.com/assets/7321975/22215935/b49ede7e-e162-11e6-98d0-ae8541e6b92e.png)

## Prerequisites

You will need:

- The **incoming webhook URL** as given by Discord.  
  Create a webhook by following the official [Discord documentation](https://support.discord.com/hc/en-us/articles/228383668-Intro-to-Webhooks). You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
- one or more Discord channels to post the messages to
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Discord

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_DISCORD` to `YES`.
2. Set `DISCORD_WEBHOOK_URL` to your webhook URL.
3. Set `DEFAULT_RECIPIENT_DISCORD` to the channel you want the alert notifications to be sent to.  
   You can define multiple channels like this: `alerts systems`.  
   All roles will default to this variable if left unconfigured.

   > ### Note
   >
   > You don't have to include the hashtag "#" of the channel, just its name.

You can then have different channels per **role**, by editing `DEFAULT_RECIPIENT_DISCORD` with the channel you want, in the following entries at the bottom of the same file:

```conf
role_recipients_discord[sysadmin]="systems"
role_recipients_discord[domainadmin]="domains"
role_recipients_discord[dba]="databases systems"
role_recipients_discord[webmaster]="marketing development"
role_recipients_discord[proxyadmin]="proxy-admin"
role_recipients_discord[sitemgr]="sites"
```

The values you provide should already exist as Discord channels in your server.

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# discord (discordapp.com) global notification options

SEND_DISCORD="YES"
DISCORD_WEBHOOK_URL="https://discord.com/api/webhooks/XXXXXXXXXXXXX/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
DEFAULT_RECIPIENT_DISCORD="alerts"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
