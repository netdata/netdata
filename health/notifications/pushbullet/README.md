# Pushbullet Agent alert notifications

Learn how to send notifications to Pushbullet using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what it will look like this on your browser:
![image](https://user-images.githubusercontent.com/70198089/229842827-e9c93e44-3c86-4ab6-9b44-d8b36a00b015.png)

And this is what it will look like on your Android device:

![image](https://user-images.githubusercontent.com/70198089/229842936-ea7e8f92-a353-43ca-a993-b1cc08e8508b.png)

## Prerequisites

You will need:

- a Pushbullet access token that can be created in your [account settings](https://www.pushbullet.com/#settings/account)
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Pushbullet

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `Send_PUSHBULLET` to `YES`.
2. Set `PUSHBULLET_ACCESS_TOKEN` to the token you generated.
3. Set `DEFAULT_RECIPIENT_PUSHBULLET` to the email (e.g. `example@domain.com`) or the channel tag (e.g. `#channel`) you want the alert notifications to be sent to.  

   > ### Note
   >
   > Please note that the Pushbullet notification service will send emails to the email recipient, regardless of if they have a Pushbullet account or not.

   You can define multiple entries like this: `user1@email.com user2@email.com`.  
   All roles will default to this variable if left unconfigured.
4. While optional, you can also set `PUSHBULLET_SOURCE_DEVICE` to the identifier of the sending device.

You can then have different recipients per **role**, by editing `DEFAULT_RECIPIENT_PUSHBULLET` with the recipients you want, in the following entries at the bottom of the same file:

```conf
role_recipients_pushbullet[sysadmin]="user1@email.com"
role_recipients_pushbullet[domainadmin]="user2@mail.com"
role_recipients_pushbullet[dba]="#channel1"
role_recipients_pushbullet[webmaster]="#channel2"
role_recipients_pushbullet[proxyadmin]="user3@mail.com"
role_recipients_pushbullet[sitemgr]="user4@mail.com"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# pushbullet (pushbullet.com) push notification options

SEND_PUSHBULLET="YES"
PUSHBULLET_ACCESS_TOKEN="XXXXXXXXX"
DEFAULT_RECIPIENT_PUSHBULLET="admin1@example.com admin3@somemail.com #examplechanneltag #anotherchanneltag"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
