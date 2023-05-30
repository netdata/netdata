# Rocket.Chat Agent alert notifications

Learn how to send notifications to Rocket.Chat using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:
![Netdata on RocketChat](https://i.imgur.com/Zu4t3j3.png)

## Prerequisites

You will need:

- The **incoming webhook URL** as given by RocketChat. You can use the same on all your Netdata servers (or you can have multiple if you like - your decision).
- one or more channels to post the messages to.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Rocket.Chat

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_ROCKETCHAT` to `YES`.
2. Set `ROCKETCHAT_WEBHOOK_URL` to your webhook URL.
3. Set `DEFAULT_RECIPIENT_ROCKETCHAT` to the channel you want the alert notifications to be sent to.  
   You can define multiple channels like this: `alerts systems`.  
   All roles will default to this variable if left unconfigured.

You can then have different channels per **role**, by editing `DEFAULT_RECIPIENT_ROCKETCHAT` with the channel you want, in the following entries at the bottom of the same file:

```conf
role_recipients_rocketchat[sysadmin]="systems"
role_recipients_rocketchat[domainadmin]="domains"
role_recipients_rocketchat[dba]="databases systems"
role_recipients_rocketchat[webmaster]="marketing development"
role_recipients_rocketchat[proxyadmin]="proxy_admin"
role_recipients_rocketchat[sitemgr]="sites"
```

The values you provide should already exist as Rocket.Chat channels.

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# rocketchat (rocket.chat) global notification options

SEND_ROCKETCHAT="YES"
ROCKETCHAT_WEBHOOK_URL="<your_incoming_webhook_url>"
DEFAULT_RECIPIENT_ROCKETCHAT="monitoring_alarms"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.

