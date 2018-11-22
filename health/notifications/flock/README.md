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
