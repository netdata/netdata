# Microsoft Teams Agent alert notifications

Learn how to send notifications to Microsoft Teams using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:
![image](https://user-images.githubusercontent.com/1122372/92710359-0385e680-f358-11ea-8c52-f366a4fb57dd.png)

## Prerequisites

You will need:

- the **incoming webhook URL** as given by Microsoft Teams. You can use the same on all your Netdata servers (or you can have multiple if you like - your decision)
- one or more channels to post the messages to
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Microsoft Teams

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_MSTEAMS` to `YES`.
2. Set `MSTEAMS_WEBHOOK_URL` to the incoming webhook URL as given by Microsoft Teams.
3. Set `DEFAULT_RECIPIENT_MSTEAMS` to the **encoded** Microsoft Teams channel name you want the alert notifications to be sent to.  
    In Microsoft Teams the channel name is encoded in the URI after `/IncomingWebhook/`.  
    You can define multiple channels like this: `CHANNEL1 CHANNEL2`.  
    All roles will default to this variable if left unconfigured.
4. You can also set the icons and colors for the different alerts in the same section of the file.

You can then have different channels per **role**, by editing `DEFAULT_RECIPIENT_MSTEAMS` with the channel you want, in the following entries at the bottom of the same file:

```conf
role_recipients_msteams[sysadmin]="CHANNEL1"
role_recipients_msteams[domainadmin]="CHANNEL2"
role_recipients_msteams[dba]="databases CHANNEL3"
role_recipients_msteams[webmaster]="CHANNEL4"
role_recipients_msteams[proxyadmin]="CHANNEL5"
role_recipients_msteams[sitemgr]="CHANNEL6"
```

The values you provide should already exist as Microsoft Teams channels in the same Team.

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Microsoft Teams (office.com) global notification options

SEND_MSTEAMS="YES"
MSTEAMS_WEBHOOK_URL="https://outlook.office.com/webhook/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX@XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX/IncomingWebhook/CHANNEL/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX"
DEFAULT_RECIPIENT_MSTEAMS="XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
