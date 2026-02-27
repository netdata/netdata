# Collector configuration

Find available collectors in the [Collecting Metrics](/src/collectors/README.md) guide and on our [Integrations page](https://www.netdata.cloud/integrations).

Each collector's documentation includes detailed setup instructions and configuration options. Most collectors either work automatically without configuration or require minimal setup to begin collecting data.

> **Info**
>
> Enable and configure Go collectors directly through the UI using the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md).

## Enable or disable Collectors and Plugins

Most collectors and plugins are enabled by default. You can selectively disable them to optimize performance.

**To disable plugins**:

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files).
2. Navigate to the `[plugins]` section
3. Uncomment the relevant line and set it to `no`

   ```text
   [plugins]
       proc = yes
       python.d = no
   ```

**To disable specific collectors**:

1. Open the corresponding plugin configuration file:
   ```bash
   sudo ./edit-config go.d.conf
   ```
2. Uncomment the collector's line and set it to `no`:
   ```yaml
   modules:
       xyz_collector: no
   ```
3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent after making changes.

## Adjust data collection frequency

You can modify how often collectors gather metrics to optimize CPU usage. This can be done globally or for specific collectors.

### Global

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files).
2. Set the `update every` value (default is `1`, meaning one-second intervals):
    ```text
    [global]
        update every = 2
    ```

3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent after making changes.

### Specific Plugin or Collector

**For Plugins**:

1. Open `netdata.conf` using [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-configuration-files).
2. Locate the plugin's section and set its frequency:

    ```text
    [plugin:apps]
        update every = 5
    ```
3. [Restart](/docs/netdata-agent/start-stop-restart.md) the Agent after making changes.

**For Collectors**:

Each collector has its own configuration format and options. Refer to the collector's documentation for specific instructions on adjusting its data collection frequency.

## Troubleshoot a collector

1. Navigate to the plugins directory. If not found, check the `plugins directory` setting in `netdata.conf`.
   ```bash
   cd /usr/libexec/netdata/plugins.d/
   ```
2. Switch to the netdata user.
   ```bash
   sudo su -s /bin/bash netdata
   ```
3. Run debug mode

   ```bash
   # Go collectors
   ./go.d.plugin -d -m <MODULE_NAME>

   # Python collectors
   ./python.d.plugin <MODULE_NAME> debug trace
   
   # Bash collectors
   ./charts.d.plugin debug 1 <MODULE_NAME>
   ```
4. Analyze output

The debug output will show:

- Configuration issues
- Connection problems
- Permission errors
- Other potential failures

Need help interpreting the results? Join our [Discord community](https://discord.com/invite/2mEmfW735j) for expert assistance.
