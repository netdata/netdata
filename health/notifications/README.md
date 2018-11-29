# Netdata alarm notifications

The `exec` line in health configuration defines an external script that will be called once
the alarm is triggered. The default script is **[alarm-notify.sh](alarm-notify.sh.in)**.

You can change the default script globally by editing `/etc/netdata/netdata.conf`.

`alarm-notify.sh` is capable of sending notifications:

- to multiple recipients
- using multiple notification methods
- filtering severity per recipient

It uses **roles**. For example `sysadmin`, `webmaster`, `dba`, etc.

Each alarm is assigned to one or more roles, using the `to` line of the alarm configuration.
Then `alarm-notify.sh` uses its own configuration file `/etc/netdata/health_alarm_notify.conf`
the default is [here](health_alarm_notify.conf)
(to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`)
to find the destination address of the notification for each method.

Each role may have one or more destinations.

So, for example the `sysadmin` role may send:

1. emails to admin1@example.com and admin2@example.com
2. pushover.net notifications to USERTOKENS `A`, `B` and `C`.
3. pushbullet.com push notifications to admin1@example.com and admin2@example.com
4. messages to slack.com channel `#alarms` and `#systems`.
5. messages to Discord channels `#alarms` and `#systems`.

## Configuration

Edit [`/etc/netdata/health_alarm_notify.conf`](health_alarm_notify.conf)
by running `/etc/netdata/edit-config health_alarm_notify.conf`:

- settings per notification method:

   all notification methods except email, require some configuration
   (i.e. API keys, tokens, destination rooms, channels, etc).

2. **recipients** per **role** per **notification method**

## Testing Notifications

You can run the following command by hand, to test alarms configuration:

```sh
# become user netdata
su -s /bin/bash netdata

# enable debugging info on the console
export NETDATA_ALARM_NOTIFY_DEBUG=1

# send test alarms to sysadmin
/usr/libexec/netdata/plugins.d/alarm-notify.sh test

# send test alarms to any role
/usr/libexec/netdata/plugins.d/alarm-notify.sh test "ROLE"
```
If you need to dig even deeper, you can trace the execution with `bash -x`. Note that in test mode, alarm-notify.sh calls itself with many more arguments. So first do
 ```sh
 bash -x /usr/libexec/netdata/plugins.d/alarm-notify.sh test
 ```
 Then look in the output for the alarm-notify.sh calls and run the one you want to trace with `bash -x`. 