# Enable an exporting connector

Now that you found the right connector for your [external time-series
database](/docs/exporting-metrics/README.md#supported-databases), you can now enable the exporting engine and the
connector itself. We'll walk through the process of enabling the exporting engine itself, followed by two examples using
the OpenTSDB and Graphite connectors.

> **Note**
>
> When you enable the exporting engine and a connector, the Netdata Agent exports metrics _beginning from the time you
> restart its process_, not the entire
> [database of long-term metrics](/src/database/README.md).

Once you understand how to enable a connector, you can apply that knowledge to any other connector.

## Enable the exporting engine

Use `edit-config` from your [Netdata config directory](/docs/netdata-agent/configuration/README.md#the-netdata-config-directory) to edit `exporting.conf`.

Enable the exporting engine itself by setting `enabled` to `yes`:

```text
[exporting:global]
    enabled = yes
```

Save the file but keep it open, as you will edit it again to enable specific connectors.

## Example: Enable the OpenTSDB connector

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`.

```text
[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
```

Replace `my_opentsdb_http_instance` with an instance name of your choice, and change the `destination` setting to the IP address or hostname of your OpenTSDB database.

[Restart your Agent](/docs/netdata-agent/start-stop-restart.md) to initiate exporting to your OpenTSDB database. The Netdata Agent continuously exports metrics collected from the moment it starts. You can expect to see data appear in your OpenTSDB database within seconds of restarting the Agent.

Any further configuration is optional, based on your needs and the configuration of your OpenTSDB database. See the [OpenTSDB connector doc](/src/exporting/opentsdb/README.md) and [exporting engine reference](/src/exporting/README.md#configuration) for details.
