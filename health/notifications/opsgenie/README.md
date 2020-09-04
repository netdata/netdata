<!--
title: "Send notifications to Opsgenie"
description: "Send alerts to your Opsgenie incident response account any time an anomaly or performance issue strikes a node in your infrastructure."
sidebar_label: "Opsgenie"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/opsgenie/README.md
-->

# Send notifications to Opsgenie

You will need an [API integration key](https://docs.opsgenie.com/docs/api-integration) to send your alarms to
[Opsgenie](https://www.atlassian.com/software/opsgenie).

## Configuration

To edit `health_alarm_notify.conf` on your system, run `/etc/netdata/edit-config health_alarm_notify.conf`.

Opsgenie has sixteen different possible [configuration options](https://docs.opsgenie.com/docs/alert-api), but only the
option `OPSGENIE_API_KEY` is required. The other options are used to improve the alarm information sent to Opsgenie.

Opsgenie has two kind of options:

-   **independent**: Variables that are not affected by the state of other variables. These are `OPSGENIE_API_KEY`,
    `OPSGENIE_ENTITY`, `OPSGENIE_ACTIONS`, and `OPSGENIE_TAGS`.
-   **types**: Variables that have values analysed in pairs, where the first value is stored in a variable named `NAME`
    and the respective pair is stored in the same position in another variable named `NAME_TYPES.`. The types variables
    are `OPSGENIE_RESPONDERS_NAMES`/`OPSGENIE_RESPONDERS_NAMES_TYPES`, `OPSGENIE_RESPONDERS_USERNAMES`/
    `OPSGENIE_RESPONDERS_USERNAMES_TYPES`, `OPSGENIE_RESPONDERS_IDS`/`OPSGENIE_RESPONDERS_IDS_TYPES`,
    `OPSGENIE_VISIBLE_NAMES`/`OPSGENIE_VISIBLE_NAMES_TYPES`, `OPSGENIE_VISIBLE_USERNAMES`/
    `OPSGENIE_VISIBLE_USERNAMES_TYPES`, and `OPSGENIE_VISIBLE_IDS`/`OPSGENIE_VISIBLE_IDS_TYPES`.

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
2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send opsgenie notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 403.
```

You can find more details about the Opsgenie error codes in their [response
docs](https://docs.opsgenie.com/docs/response).

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fhealth%2Fnotifications%2Fopsgenie%2FREADME%2FDonations-netdata-has-received&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
