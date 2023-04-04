# Twilio Agent alert notifications

Learn how to send notifications to Twilio using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

Will look like this on your Android device:

![image](https://user-images.githubusercontent.com/70198089/229841323-6c4b1956-dd91-423e-abaf-2799000f72a8.png)


## Prerequisites

You will need:

- to get your SID, and Token from <https://www.twilio.com/console>
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Twilio

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_TWILIO` to `YES`.
2. Set `TWILIO_ACCOUNT_SID` to your account SID.
3. Set `TWILIO_ACCOUNT_TOKEN` to your account token.
4. Set `TWILIO_NUMBER` to your account's number.
5. Set `DEFAULT_RECIPIENT_TWILIO` to the number you want the alert notifications to be sent to.  
   You can define multiple numbers like this: `+15555555555 +17777777777`.  
   All roles will default to this variable if left unconfigured.

    > ### Note
    >
    > Please not that if your account is a trial account you will only be able to send notifications to the number you signed up with.

You can then have different recipients per **role**, by editing `DEFAULT_RECIPIENT_TWILIO` with the recipient's number you want, in the following entries at the bottom of the same file:

```conf
role_recipients_twilio[sysadmin]="+15555555555"
role_recipients_twilio[domainadmin]="+15555555556"
role_recipients_twilio[dba]="+15555555557"
role_recipients_twilio[webmaster]="+15555555558"
role_recipients_twilio[proxyadmin]="+15555555559"
role_recipients_twilio[sitemgr]="+15555555550"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Twilio (twilio.com) SMS options

SEND_TWILIO="YES"
TWILIO_ACCOUNT_SID="xxxxxxxxx"
TWILIO_ACCOUNT_TOKEN="xxxxxxxxxx"
TWILIO_NUMBER="xxxxxxxxxxx"
DEFAULT_RECIPIENT_TWILIO="+15555555555"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
