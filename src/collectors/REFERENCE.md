# Collector configuration

The list of supported collectors can be found in the [Collecting Metrics](/src/collectors/README.md) section,
and on [our website](https://www.netdata.cloud/integrations).

The documentation of each Collector provides all the necessary configuration options and prerequisites for that collector. In most cases, either the charts are automatically generated without any configuration, or you fulfil those prerequisites and configure the collector.

> **Info**
>
> You can enable and configure Go Collectors using the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md) from the UI.

## Enable or disable Collectors and Plugins

By default, most Collectors and Plugins are enabled, but you might want to disable something specific for optimization purposes.

Using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config), open `netdata.conf` and scroll down to the `[plugins]` section.

To disable a plugin, uncomment the line and set the value to `no`.

```text
[plugins]
    proc = yes
    python.d = no
```

Disable specific collectors by opening their respective plugin configuration files, uncommenting the line for that collector, and setting its value to `no`.

```bash
sudo ./edit-config go.d.conf
```

```text
modules:
   xyz_collector: no
```

After you make your changes, restart the Agent with the [appropriate method](/docs/netdata-agent/start-stop-restart.md) for your system.

## Adjust data collection frequency

In some scenarios, you might want to increase or decrease the data collection frequency of the Collectors as it directly affects CPU utilization.

### Global

Using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) open `netdata.conf` and edit the `update every` value.

The default is `1`, meaning that the Agent collects metrics every second.

> **Note**
>
> If the `update every` for an individual collector is less than the global, the Netdata Agent uses the global setting.
> If you change this to `2`, Netdata enforces a minimum `update every` setting of 2 seconds, and collects metrics every other second, which will effectively halve CPU utilization.

```text
[global]
    update every = 2
```

Set this to `5` or `10` to collect metrics every 5 or 10 seconds, respectively.

After you make your changes, restart the Agent with the [appropriate method](/docs/netdata-agent/start-stop-restart.md) for your system.

### Specific Plugin or Collector

Every Collector and Plugin have their own `update every` settings, which you can also change in their respective configuration files.

To reduce the collection frequency of a plugin, open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) and find the appropriate section.

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

## Troubleshoot a collector

First, navigate to your plugins directory, which is usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the plugins directory, switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

The next step is based on the collector's orchestrator.

```bash
# Go orchestrator (go.d.plugin)
./go.d.plugin -d -m <MODULE_NAME>

# Python orchestrator (python.d.plugin)
./python.d.plugin <MODULE_NAME> debug trace

# Bash orchestrator (bash.d.plugin)
./charts.d.plugin debug 1 <MODULE_NAME>
```

The output from the relevant command will provide valuable troubleshooting information. If you can't figure out how to enable the Collector using the details from this output, feel free to [join our Discord server](https://discord.com/invite/2mEmfW735j), to get help from our experts.
