# profile.plugin

This plugin allows someone to backfill an Agent with random data.

A user can specify:

- The number charts they want,
- the number of dimensions per chart,
- the desire update every collection frequency,
- the number of seconds to backfill.
- the number of collection threads.

## Configuration

Edit the `netdata.conf` configuration file using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files) from the [Netdata config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory), which is typically at `/etc/netdata`.

Scroll down to the `[plugin:profile]` section to find the available options:

```text
[plugin:profile]
    update every = 5
    number of charts = 200
    number of dimensions per chart = 5
    seconds to backfill = 86400
    number of threads = 16
```

The `number of threads` option will create the specified number of collection
threads. The rest of the options apply to each thread individually, eg. the
above configuration will create 3200 charts, 16000 dimensions in total, which will be
backfilled for the duration of 1 day.

Note that all but the 1st chart created in each thread will be marked as hidden
in order to ease the load on the dashboard's UI.
