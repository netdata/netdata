# Alarm notifications

Each netdata alarm can have its own alarm action.

The default alarm action is the script [`alarm-notify.sh`](https://github.com/netdata/netdata/blob/master/plugins.d/alarm-notify.sh).

This script has the following features:

1. The `to` line of alarms can list one or more **roles**.
2. Each **role** can correspond to multiple **recipients**, using multiple **notification methods** (email, slack, discord, twilio, pagerduty, etc).
3. Each **recipient** can filter severity (i.e. receive only critical notifications).
4. The special role `silent` can be used in alarms that should not send any kind of notification.

## Notifications Configuration

Edit [`/etc/netdata/health_alarm_notify.conf`](https://github.com/netdata/netdata/blob/master/conf.d/health_alarm_notify.conf) to configure:

1. settings per notification method

   all notification methods except email, require some configuration (i.e. API keys, tokens, destination rooms, channels, etc).

2. **recipients** per **role** per **notification method**


See also [alarm actions](https://github.com/netdata/netdata/wiki/health-configuration-reference#alarm-actions).


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
