# Pushover Agent alert notifications

Learn how to send notification to Pushover using Netdata's Agent alert notification
feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:  

![image](https://user-images.githubusercontent.com/70198089/229842244-4ac998bb-6158-4955-ac2d-766a9999cc98.png)

Netdata will send warning messages with priority `0` and critical messages with priority `1`. Pushover allows you to select do-not-disturb hours. The way this is configured, critical notifications will ring and vibrate your phone, even during the do-not-disturb-hours. All other notifications will be delivered silently.

## Prerequisites

You will need:

- An Application token. You can use the same on all your Netdata servers.
- A User token for each user you are going to send notifications to. This is the actual recipient of the notification.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Pushover

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_PUSHOVER` to `YES`.
2. Set `PUSHOVER_APP_TOKEN` to your Pushover Application token.
3. Set `DEFAULT_RECIPIENT_PUSHOVER` to the Pushover User token you want the alert notifications to be sent to.  
   You can define multiple User tokens like this: `USERTOKEN1 USERTOKEN2`.  
   All roles will default to this variable if left unconfigured.

You can then have different User tokens per **role**, by editing `DEFAULT_RECIPIENT_PUSHOVER` with the token you want, in the following entries at the bottom of the same file:

```conf
role_recipients_pushover[sysadmin]="USERTOKEN1"
role_recipients_pushover[domainadmin]="USERTOKEN2"
role_recipients_pushover[dba]="USERTOKEN3 USERTOKEN4"
role_recipients_pushover[webmaster]="USERTOKEN5"
role_recipients_pushover[proxyadmin]="USERTOKEN6"
role_recipients_pushover[sitemgr]="USERTOKEN7"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# pushover (pushover.net) global notification options

SEND_PUSHOVER="YES"
PUSHOVER_APP_TOKEN="XXXXXXXXX"
DEFAULT_RECIPIENT_PUSHOVER="USERTOKEN"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
