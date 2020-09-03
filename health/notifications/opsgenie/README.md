<!--
---
title: "Opsgenie"
custom_edit_url: https://github.com/netdata/netdata/edit/master/health/notifications/opsgenie/README.md
---
-->

# Opsgenie

You will need an [API integration key](https://docs.opsgenie.com/docs/api-integration) to send your alarms for Opsgenie.

## configuration

To edit `health_alarm_notify.conf` on your system run `/etc/netdata/edit-config health_alarm_notify.conf`.

Opsgenie has sixteen different possible configuration [options](https://docs.opsgenie.com/docs/alert-api), but only the
option `OPSGENIE_API_KEY` is obligatory, the other options are used to improve information delivered with alarm.

Opsgenie has two kind of options:

-   independent: Variables that are treated without to consider other variables, they are `OPSGENIE_API_KEY`,
 `OPSGENIE_ENTITY`, `OPSGENIE_ACTIONS` and `OPSGENIE_TAGS`.
-   types: Variables that have values analysed in pairs, where the first value is stored in a variable named `NAME` and
the respective pair is stored in the same position in another variable named `NAME_TYPES.`. The types variables are
`OPSGENIE_RESPONDERS_NAMES` and `OPSGENIE_RESPONDERS_NAMES_TYPES`, `OPSGENIE_RESPONDERS_USERNAMES` and
`OPSGENIE_RESPONDERS_USERNAMES_TYPES`, `OPSGENIE_RESPONDERS_IDS` and `OPSGENIE_RESPONDERS_IDS_TYPES`, 
`OPSGENIE_VISIBLE_NAMES` and `OPSGENIE_VISIBLE_NAMES_TYPES`, `OPSGENIE_VISIBLE_USERNAMES` and
`OPSGENIE_VISIBLE_USERNAMES_TYPES`, `OPSGENIE_VISIBLE_IDS` and `OPSGENIE_VISIBLE_IDS_TYPES`.

Changes to this file do not require a Netdata restart.

You can test your configuration by issuing the commands:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# send a test alarm
/usr/libexec/netdata/plugins.d/alarm-notify.sh test [ROLE]
```

On success you will have alarms in your Opsgenie platform

![image](https://user-images.githubusercontent.com/49162938/92184518-f725f900-ee40-11ea-9afa-e7c639c72206.png)

, but if Netdata receives an error message, it will print a message like 

```
2020-09-03 23:07:00: alarm-notify.sh: ERROR: failed to send opsgenie notification for: hades test.chart.test_alarm is CRITICAL, with HTTP error code 403.
```

and you can obtain more details in this [link](https://docs.opsgenie.com/docs/response).