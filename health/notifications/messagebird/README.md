# MessageBird Agent alert notifications

Learn how to send notifications to MessageBird using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:

![image](https://user-images.githubusercontent.com/70198089/229841323-6c4b1956-dd91-423e-abaf-2799000f72a8.png)

## Prerequisites

You will need:

- an access key under 'API ACCESS (REST)' (you will want a live key), you can read more [here](https://developers.messagebird.com/quickstarts/sms/test-credits-api-keys/)
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to MessageBird

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_MESSAGEBIRD` to `YES`.
2. Set `MESSAGEBIRD_ACCESS_KEY` to your API access key.
3. Set `MESSAGEBIRD_NUMBER` to the MessageBird number you want to use for the alert.
4. Set `DEFAULT_RECIPIENT_MESSAGEBIRD` to the number you want the alert notification to be sent as an SMS.  
  You can define multiple recipients like this: `+15555555555 +17777777777`.  
  All roles will default to this variable if left unconfigured.

You can then have different recipients per **role**, by editing `DEFAULT_RECIPIENT_MESSAGEBIRD` with the number you want, in the following entries at the bottom of the same file:

```conf
role_recipients_messagebird[sysadmin]="+15555555555"
role_recipients_messagebird[domainadmin]="+15555555556"
role_recipients_messagebird[dba]="+15555555557"
role_recipients_messagebird[webmaster]="+15555555558"
role_recipients_messagebird[proxyadmin]="+15555555559"
role_recipients_messagebird[sitemgr]="+15555555550"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Messagebird (messagebird.com) SMS options

SEND_MESSAGEBIRD="YES"
MESSAGEBIRD_ACCESS_KEY="XXXXXXXX"
MESSAGEBIRD_NUMBER="XXXXXXX"
DEFAULT_RECIPIENT_MESSAGEBIRD="+15555555555"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
