# SMS Server Tools 3 Agent alert notifications

Learn how to send notifications to `smstools3` using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

The [SMS Server Tools 3](http://smstools3.kekekasvi.com/) is a SMS Gateway software which can send and receive short messages through GSM modems and mobile phones.

## Prerequisites

You will need:

- to [install](http://smstools3.kekekasvi.com/index.php?p=compiling) and [configure](http://smstools3.kekekasvi.com/index.php?p=configure) smsd

- To ensure that the user `netdata` can execute `sendsms`. Any user executing `sendsms` needs to:
  - have write permissions to `/tmp` and `/var/spool/sms/outgoing`
  - be a member of group `smsd`

    To ensure that the steps above are successful, just `su netdata` and execute `sendsms phone message`.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to SMS Server Tools 3

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set the path for `sendsms`, otherwise Netdata will search for it in your system `$PATH`:

    ```conf
    # The full path of the sendsms command (smstools3).
    # If empty, the system $PATH will be searched for it.
    # If not found, SMS notifications will be silently disabled.
    sendsms="/usr/bin/sendsms"
    ```

2. Set `SEND_SMS` to `YES`.
3. Set `DEFAULT_RECIPIENT_SMS` to the phone number you want the alert notifications to be sent to.  
    You can define multiple phone numbers like this: `PHONE1 PHONE2`.  
    All roles will default to this variable if left unconfigured.

You can then have different phone numbers per **role**, by editing `DEFAULT_RECIPIENT_IRC` with the phone number you want, in the following entries at the bottom of the same file:

```conf
role_recipients_sms[sysadmin]="PHONE1"
role_recipients_sms[domainadmin]="PHONE2"
role_recipients_sms[dba]="PHONE3"
role_recipients_sms[webmaster]="PHONE4"
role_recipients_sms[proxyadmin]="PHONE5"
role_recipients_sms[sitemgr]="PHONE6"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# SMS Server Tools 3 (smstools3) global notification options
SEND_SMS="YES"
DEFAULT_RECIPIENT_SMS="1234567890"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
