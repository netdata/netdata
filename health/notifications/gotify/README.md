<!--
title: "Gotify agent alert notifications"
description: "Send alerts to your Gotify instance when an alert gets triggered in Netdata."
sidebar_label: "Gotify"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/gotify/README.md
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Integrations/Notify/Agent alert notifications"
learn_autogeneration_metadata: "{'part_of_cloud': False, 'part_of_agent': True}"
-->

# Gotify agent alert notifications

[Gotify](https://gotify.net/) is a self-hosted push notification service created for sending and receiving messages in real time.

## Configuring Gotify

### Prerequisites

To use Gotify as your notification service, you need an application token. 
You can generate a new token in the Gotify Web UI. 

### Configuration

To set up Gotify in Netdata: 

1. Switch to your [config
directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md) and edit the file `health_alarm_notify.conf` using the edit config script.
 
   ```bash
   ./edit-config health_alarm_notify.conf
   ```

2. Change the variable `GOTIFY_APP_TOKEN` to the application token you generated in the Gotify Web UI. Change
`GOTIFY_APP_URL` to point to your Gotify instance.

   ```conf
   SEND_GOTIFY="YES"

   # Application token
   # Gotify instance url
   GOTIFY_APP_TOKEN=XXXXXXXXXXXXXXX
   GOTIFY_APP_URL=https://push.example.de/
   ```

   Changes to `health_alarm_notify.conf` do not require a Netdata restart. 
   
3. Test your Gotify notifications configuration by running the following commands, replacing `ROLE` with your preferred role:

   ```sh
   # become user netdata
   sudo su -s /bin/bash netdata

   # send a test alarm
   /usr/libexec/netdata/plugins.d/alarm-notify.sh test ROLE
   ```

   🟢 If everything works, you'll see alarms in Gotify:

   ![Example alarm notifications in Gotify](https://user-images.githubusercontent.com/103264516/162509205-1e88e5d9-96b6-4f7f-9426-182776158128.png)

   🔴 If sending the test notifications fails, check `/var/log/netdata/error.log` to find the relevant error message:

   ```log 
   2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send Gotify notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 401.
   ```
