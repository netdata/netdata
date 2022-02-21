<!--
title: "Send notifications to Opsgenie"
description: "Send alerts to your Opsgenie incident response account any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "Opsgenie"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/opsgenie/README.md
-->

# Send notifications to Opsgenie

[Opsgenie](https://www.atlassian.com/software/opsgenie) is an alerting and incident response tool. It is designed to
group and filter alarms, build custom routing rules for on-call teams, and correlate deployments and commits to
incidents.

The first step is to create a [Netdata integration](https://docs.opsgenie.com/docs/api-integration) in the
[Opsgenie](https://www.atlassian.com/software/opsgenie) dashboard. After this, you need to edit
`health_alarm_notify.conf` on your system, by running the following from your [config
directory](/docs/configure/nodes.md):
 
```bash
./edit-config health_alarm_notify.conf
```

Change the variable `OPSGENIE_API_KEY` with the API key you got from Opsgenie. `OPSGENIE_API_URL` defaults to
`https://api.opsgenie.com`, however there are region-specific API URLs such as `https://eu.api.opsgenie.com`, so set
this if required.

```conf
SEND_OPSGENIE="YES"

# Api key
# Default Opsgenie API
OPSGENIE_API_KEY="11111111-2222-3333-4444-555555555555"
OPSGENIE_API_URL=""
```

Changes to `health_alarm_notify.conf` do not require a Netdata restart. You can test your Opsgenie notifications
configuration by issuing the commands, replacing `ROLE` with your preferred role:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test ROLE
```

If everything works, you'll see alarms in your Opsgenie platform:

![Example alarm notifications in
Opsgenie](https://user-images.githubusercontent.com/49162938/92184518-f725f900-ee40-11ea-9afa-e7c639c72206.png)

If sending the test notifications fails, you can look in `/var/log/netdata/error.log` to find the relevant error
message:

```log
2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send opsgenie notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 401.
```

You can find more details about the Opsgenie error codes in their [response
docs](https://docs.opsgenie.com/docs/response).


