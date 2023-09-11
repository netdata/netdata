# Kavenegar Agent alert notifications

Learn how to send notifications to Kavenegar using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

[Kavenegar](https://kavenegar.com/) as service for software developers, based in Iran, provides send and receive SMS, calling voice by using its APIs.

This is what you will get:

![image](https://user-images.githubusercontent.com/70198089/229841323-6c4b1956-dd91-423e-abaf-2799000f72a8.png)

## Prerequisites

You will need:

- the `APIKEY` and Sender from <http://panel.kavenegar.com/client/setting/account>
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Kavenegar

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SEND_KAVENEGAR` to `YES`.
2. Set `KAVENEGAR_API_KEY` to your `APIKEY`.
3. Set `KAVENEGAR_SENDER` to the value of your Sender.
4. Set `DEFAULT_RECIPIENT_KAVENEGAR` to the SMS recipient you want the alert notifications to be sent to.  
   You can define multiple recipients like this: `09155555555 09177777777`.  
   All roles will default to this variable if lest unconfigured.

You can then have different SMS recipients per **role**, by editing `DEFAULT_RECIPIENT_KAVENEGAR` with the SMS recipients you want, in the following entries at the bottom of the same file:

```conf
role_recipients_kavenegar[sysadmin]="09100000000"
role_recipients_kavenegar[domainadmin]="09111111111"
role_recipients_kavenegar[dba]="0922222222"
role_recipients_kavenegar[webmaster]="0933333333"
role_recipients_kavenegar[proxyadmin]="0944444444"
role_recipients_kavenegar[sitemgr]="0955555555"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# Kavenegar (Kavenegar.com) SMS options

SEND_KAVENEGAR="YES"
KAVENEGAR_API_KEY="XXXXXXXXXXXX"
KAVENEGAR_SENDER="YYYYYYYY"
DEFAULT_RECIPIENT_KAVENEGAR="0912345678"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
