<!--
title: "Send notifications to Opsgenie"
description: "Send alerts to your Opsgenie incident response account any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "Opsgenie"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/opsgenie/README.md
-->

# Send notifications to Opsgenie

The first step is to create a [Netdata integration](https://docs.opsgenie.com/docs/api-integration) on
[Opsgenie](https://www.atlassian.com/software/opsgenie) dashboard. After this it is necessary to edit 
 `health_alarm_notify.conf` on your system, run the command 
 
```bash
# /etc/netdata/edit-config health_alarm_notify.conf`
```

and change the variable `OPSGENIE_API_KEY`:

```
SEND_OPSGENIE="YES"

# Api key
# Default Opsgenie APi
OPSGENIE_API_KEY="11111111-2222-3333-4444-555555555555"

```

Changes to `health_alarm_notify.conf` do not require a Netdata restart.

You can test your Opsgenie notifications configuration by issuing the commands:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test [ROLE]
```

If everything works, you'll see alarms in your Opsgenie platform:

![Example alarm notifications in
Opsgenie](https://user-images.githubusercontent.com/49162938/92184518-f725f900-ee40-11ea-9afa-e7c639c72206.png)

If sending the test notifications fails, you can look in `/var/log/netdata/error.log` to find the relevant error
message:

```log
2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send opsgenie notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 401.
```

You can find more details about the Opsgenie error codes in their [response docs](https://docs.opsgenie.com/docs/response).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fopsgenie%2FREADME%2FDonations-netdata-has-received&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
