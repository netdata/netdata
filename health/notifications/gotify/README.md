<!--
title: "Send notifications to Gotify"
description: "Send alerts to your Gotify instance when an alert gets triggered in Netdata."
sidebar_label: "Gotify"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/gotify/README.md
-->

# Send notifications to Gotify

[Gotify](https://gotify.net/) is a self-hosted push notification service created for sending and receiving messages in real time.

First you have to create an application token in the Gotify WebUI. After you have generated a token, edit
`health_alarm_notify.conf` on your system, by running the following from your [config
directory](/docs/configure/nodes.md):
 
```bash
./edit-config health_alarm_notify.conf
```

Change the variable `GOTIFY_APP_TOKEN` with the application token you generated in Gotify. You have to also change
`GOTIFY_APP_URL` to point to your Gotify instance.

```conf
SEND_GOTIFY="YES"

# Application token
# Gotify instance url
GOTIFY_APP_TOKEN=Ar-6M777HvDPylv
GOTIFY_APP_URL=https://push.example.de/
```

Changes to `health_alarm_notify.conf` do not require a Netdata restart. You can test your Gotify notifications
configuration by issuing the commands, replacing `ROLE` with your preferred role:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test ROLE
```

If everything works, you'll see alarms in your Gotify platform:

![Example alarm notifications in
Gotify](https://user-images.githubusercontent.com/103264516/162509205-1e88e5d9-96b6-4f7f-9426-182776158128.png)

If sending the test notifications fails, you can look in `/var/log/netdata/error.log` to find the relevant error
message:

```log
2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send Gotify notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 401.
```
