# Data collection frequency

In some scenarios you might want to increase or decrease the data collection frequency of the Collectors as it directly affects CPU utilization.

## Global

Using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) open `netdata.conf` and edit the `update every` value.

The default is `1`, meaning that the Agent collects metrics every second.

> **Note**
>
> If the `update every` for an individual collector is less than the global, the Netdata Agent uses the global setting.

If you change this to `2`, Netdata enforces a minimum `update every` setting of 2 seconds, and collects metrics every other second, which will effectively halve CPU utilization.

```text
[global]
    update every = 2
```

Set this to `5` or `10` to collect metrics every 5 or 10 seconds, respectively.

After you make your changes, restart the Agent with the [appropriate method](/docs/netdata-agent/start-stop-restart.md) for your system.

## Specific Plugin or Collector

Every Collector and Plugin have their own `update every` settings, which you can also change in their respective configuration files.

To reduce the collection frequency of a [Plugin](/src/collectors/README.md#collector-architecture-and-terminology), open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) and find the appropriate section.

For example, to reduce the frequency of the `apps` plugin:

```text
[plugin:apps]
    update every = 5
```

To reduce collection frequency of a Collector, open its configuration file using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) and look for the `update_every` setting.

For example, to reduce the frequency of the `nginx` collector, run `sudo ./edit-config go.d/nginx.conf` and if not there, add:

```text
update_every: 20

jobs:
...
```

After you make your changes, restart the Agent with the [appropriate method](/docs/netdata-agent/start-stop-restart.md) for your system.
