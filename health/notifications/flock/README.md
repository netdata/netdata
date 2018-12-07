# flock.com

This is what you will get:


![Flock](https://i.imgur.com/ok9bRzw.png)

You need:

The **incoming webhook URL** as given by flock.com. You can use the same on all your netdata servers (or you can have multiple if you like - your decision).

Get them here: https://admin.flock.com/webhooks

Set them in `/etc/netdata/health_alarm_notify.conf` (to edit it on your system run `/etc/netdata/edit-config health_alarm_notify.conf`), like this:

```
###############################################################################
# sending flock notifications

# enable/disable sending pushover notifications
SEND_FLOCK="YES"

# Login to flock.com and create an incoming webhook.
# You need only one for all your netdata servers.
# Without it, netdata cannot send flock notifications.
FLOCK_WEBHOOK_URL="https://api.flock.com/hooks/sendMessage/XXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX"

# if a role recipient is not configured, no notification will be sent
DEFAULT_RECIPIENT_FLOCK="alarms"

```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fflock%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
