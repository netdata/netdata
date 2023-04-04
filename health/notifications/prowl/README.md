# Prowl Agent alert notifications

Learn how to send notifications to Prowl using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[Prowl](https://www.prowlapp.com/) is a push notification service for iOS devices.  
Netdata supports delivering notifications to iOS devices through Prowl.

Because of how Netdata integrates with Prowl, there is a hard limit of
at most 1000 notifications per hour (starting from the first notification
sent). Any alerts beyond the first thousand in an hour will be dropped.

Warning messages will be sent with the 'High' priority, critical messages
will be sent with the 'Emergency' priority, and all other messages will
be sent with the normal priority.  Opening the notification's associated
URL will take you to the Netdata dashboard of the system that issued
the alert, directly to the chart that it triggered on.

## Prerequisites

You will need:

- a Prowl API key, which can be requested through the Prowl website after registering
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Prowl

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_PROWL` to `YES`.
2. Set `DEFAULT_RECIPIENT_PROWL` to the Prowl API key you want the alert notifications to be sent to.  
   You can define multiple API keys like this: `APIKEY1, APIKEY2`.  
   All roles will default to this variable if left unconfigured.

You can then have different API keys per **role**, by editing `DEFAULT_RECIPIENT_PROWL` with the API keys you want, in the following entries at the bottom of the same file:

```conf
role_recipients_prowl[sysadmin]="AAAAAAAA"
role_recipients_prowl[domainadmin]="BBBBBBBBB"
role_recipients_prowl[dba]="CCCCCCCCC"
role_recipients_prowl[webmaster]="DDDDDDDDDD"
role_recipients_prowl[proxyadmin]="EEEEEEEEEE"
role_recipients_prowl[sitemgr]="FFFFFFFFFF"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# iOS Push Notifications

SEND_PROWL="YES"
DEFAULT_RECIPIENT_PROWL="XXXXXXXXXX"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
