# IRC Agent alert notifications

Learn how to send notifications to IRC using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

This is what you will get:

IRCCloud web client:  
![image](https://user-images.githubusercontent.com/31221999/36793487-3735673e-1ca6-11e8-8880-d1d8b6cd3bc0.png)

Irssi terminal client:  
![image](https://user-images.githubusercontent.com/31221999/36793486-3713ada6-1ca6-11e8-8c12-70d956ad801e.png)

## Prerequisites

You will need:

- The `nc` utility.  
   You can set the path to it, or Netdata will search for it in your system `$PATH`.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to IRC

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set the path for `nc`, otherwise Netdata will search for it in your system `$PATH`:

    ```conf
    #------------------------------------------------------------------------------
    # external commands
    #
    # The full path of the nc command.
    # If empty, the system $PATH will be searched for it.
    # If not found, irc notifications will be silently disabled.
    nc="/usr/bin/nc"
    ```

2. Set `SEND_IRC` to `YES`
3. Set `DEFAULT_RECIPIENT_IRC` to one or more channels to post the messages to.  
   You can define multiple channels like this: `#alarms #systems`.  
   All roles will default to this variable if left unconfigured.
4. Set `IRC_NETWORK` to the IRC network which your preferred channels belong to.
5. Set `IRC_PORT` to the IRC port to which a connection will occur.
6. Set `IRC_NICKNAME` to the IRC nickname which is required to send the notification.  
   It must not be an already registered name as the connection's `MODE` is defined as a `guest`.
7. Set `IRC_REALNAME` to the IRC realname which is required in order to make he connection.

You can then have different channels per **role**, by editing `DEFAULT_RECIPIENT_IRC` with the channel you want, in the following entries at the bottom of the same file:

```conf
role_recipients_irc[sysadmin]="#systems"
role_recipients_irc[domainadmin]="#domains"
role_recipients_irc[dba]="#databases #systems"
role_recipients_irc[webmaster]="#marketing #development"
role_recipients_irc[proxyadmin]="#proxy-admin"
role_recipients_irc[sitemgr]="#sites"
```

The values you provide should be IRC channels which belong to the specified IRC network.

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# irc notification options
#
SEND_IRC="YES"
DEFAULT_RECIPIENT_IRC="#system-alarms"
IRC_NETWORK="irc.freenode.net"
IRC_NICKNAME="netdata-alarm-user"
IRC_REALNAME="netdata-user"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
