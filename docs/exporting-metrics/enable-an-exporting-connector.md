# Enable an Exporting Connector

After selecting the right connector for your [external time-series database](/docs/exporting-metrics/README.md#supported-databases), you can enable the exporting engine and configure your connector. This guide walks through enabling the exporting engine itself, followed by two examples using the OpenTSDB and Graphite connectors.

:::info

When you enable the exporting engine and a connector, Netdata exports metrics **starting from the Agent restart time**, not the entire [historical database](/src/database/README.md).

:::

Once you understand how to enable a connector, you can apply that knowledge to any other connector.

## Enable the Exporting Engine

Edit `exporting.conf` using `edit-config` from your [Netdata config directory](/docs/netdata-agent/configuration/README.md#edit-configuration-files):

```text
[exporting:global]
    enabled = yes
```

Save the file but keep it open—you will edit it again to enable specific connectors.

## Examples

<details>
<summary><strong>Enable the OpenTSDB Connector</strong></summary>

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`:

```text
[opentsdb:http:my_opentsdb_http_instance]
    enabled = yes
    destination = localhost:4242
```

Replace `my_opentsdb_http_instance` with an instance name of your choice, and change the `destination` setting to the IP address or hostname of your OpenTSDB database.

[Restart your Agent](/docs/netdata-agent/start-stop-restart.md) to initiate exporting to your OpenTSDB database. The Netdata Agent continuously exports metrics collected from the moment it starts. You can expect to see data appear in your OpenTSDB database within seconds of restarting the Agent.

Any further configuration is optional, based on your needs and the configuration of your OpenTSDB database. See the [OpenTSDB connector doc](/src/exporting/opentsdb/README.md) and [exporting engine reference](/src/exporting/README.md#configuration-structure) for details.

</details>

<details>
<summary><strong>Enable the Graphite Connector</strong></summary>

Use the following configuration as a starting point. Copy and paste it into `exporting.conf`:

```text
[graphite:netdata]
    enabled = yes
    destination = localhost:2003
```

Replace `netdata` with an instance name of your choice, and change the `destination` setting to the IP address or hostname of your Graphite database.

[Restart your Agent](/docs/netdata-agent/start-stop-restart.md) to initiate exporting to your Graphite database. The Netdata Agent continuously exports metrics collected from the moment it starts. You can expect to see data appear in your Graphite database within seconds of restarting the Agent.

Any further configuration is optional, based on your needs and the configuration of your Graphite database. See the [Graphite connector doc](/src/exporting/graphite/README.md) and [exporting engine reference](/src/exporting/README.md#configuration-structure) for details.

</details>
