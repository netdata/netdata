# Syslog Agent alert notifications

Learn how to send notifications to Syslog using Netdata's Agent alert notification feature, which supports dozens of endpoints, user roles, and more.

> ### Note
>
> This file assumes you have read the [Introduction to Agent alert notifications](https://github.com/netdata/netdata/blob/master/health/notifications/README.md), detailing how the Netdata Agent's alert notification method works.

Logged messages will look like this:

```bash
netdata WARNING on hostname at Tue Apr 3 09:00:00 EDT 2018: disk_space._ out of disk space time = 5h
```

## Prerequisites

You will need:

- A working `logger` command for this to work. This is the case on pretty much every Linux system in existence, and most BSD systems.
- terminal access to the Agent you wish to configure

## Configure Netdata to send alert notifications to Syslog

> ### Info
>
> This file mentions editing configuration files.  
>
> - To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.  
> Note that to run the script you need to be inside your Netdata config directory.
>
> It is recommended to use this way for configuring Netdata.

Edit `health_alarm_notify.conf`, changes to this file do not require restarting Netdata:

1. Set `SYSLOG_FACILITY` to the facility used for logging, by default this value is set to `local6`.
2. Set `DEFAULT_RECIPIENT_SYSLOG` to the recipient you want the alert notifications to be sent to.  
    Targets are defined as follows:

    ```conf
    [[facility.level][@host[:port]]/]prefix
    ```

    `prefix` defines what the log messages are prefixed with.  By default, all lines are prefixed with 'netdata'.

    The `facility` and `level` are the standard syslog facility and level options, for more info on them see your local `logger` and `syslog` documentation.  By default, Netdata will log to the `local6` facility, with a log level dependent on the type of message (`crit` for CRITICAL, `warning` for WARNING, and `info` for everything else).

    You can configure sending directly to remote log servers by specifying a host (and optionally a port).  However, this has a somewhat high overhead, so it is much preferred to use your local syslog daemon to handle the forwarding of messages to remote systems (pretty much all of them allow at least simple forwarding, and most of the really popular ones support complex queueing and routing of messages to remote log servers).

    You can define multiple recipients like this: `daemon.notice@loghost:514/netdata daemon.notice@loghost2:514/netdata`.  
    All roles will default to this variable if left unconfigured.
3. Lastly, set `SEND_SYSLOG` to `YES`, make sure you have everything else configured _before_ turning this on.

You can then have different recipients per **role**, by editing `DEFAULT_RECIPIENT_SYSLOG` with the recipient you want, in the following entries at the bottom of the same file:

```conf
role_recipients_syslog[sysadmin]="daemon.notice@loghost1:514/netdata"
role_recipients_syslog[domainadmin]="daemon.notice@loghost2:514/netdata"
role_recipients_syslog[dba]="daemon.notice@loghost3:514/netdata"
role_recipients_syslog[webmaster]="daemon.notice@loghost4:514/netdata"
role_recipients_syslog[proxyadmin]="daemon.notice@loghost5:514/netdata"
role_recipients_syslog[sitemgr]="daemon.notice@loghost6:514/netdata"
```

An example of a working configuration would be:

```conf
#------------------------------------------------------------------------------
# syslog notifications

SEND_SYSLOG="YES"
SYSLOG_FACILITY='local6'
DEFAULT_RECIPIENT_SYSLOG="daemon.notice@loghost6:514/netdata"
```

## Test the notification method

To test this alert notification method refer to the ["Testing Alert Notifications"](https://github.com/netdata/netdata/blob/master/health/notifications/README.md#testing-alert-notifications) section of the Agent alert notifications page.
