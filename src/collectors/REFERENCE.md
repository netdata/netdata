# Collector configuration

The list of supported collectors can be found in the [Collecting Metrics](/src/collectors/README.md) section,
and on [our website](https://www.netdata.cloud/integrations).

The documentation of each Collector provides all the necessary configuration options and prerequisites for that collector. In most cases, either the charts are automatically generated without any configuration, or you just fulfil those prerequisites and [configure the collector](#configure-a-collector).

You can enable and configure Go Collectors using the [Dynamic Configuration Manager](/docs/netdata-agent/configuration/dynamic-configuration.md) from the UI.

## Configure a collector

Most Collector modules come with **auto-detection**, configured to work out-of-the-box on popular operating systems with
the default settings.

However, there are cases that auto-detection is not possible. Usually, the reason is that the applications to be monitored do not allow Netdata to connect, or are custom-configured and expose metrics to a custom port, etc.

In most of the cases, allowing the user `netdata` from `localhost` to connect and collect metrics, will automatically enable data collection for the application in question (it will require a Netdata restart).

When Netdata starts up, each Collector searches for exposed metrics on the default endpoint established by that service
or application's standard installation procedure. For example, the [Nginx collector](/src/go/plugin/go.d/modules/nginx/README.md) searches at `http://127.0.0.1/stub_status` for exposed metrics in the correct format. If an Nginx web server is running and exposes metrics on that endpoint, the Collector begins gathering them.

However, not every node or infrastructure uses standard ports, paths, files, or naming conventions and some Collectors need credentials to work.

First, [find the collector](/src/collectors/README.md) you want to edit and open its documentation. On that page you will find all the necessary instructions to properly configure the Collector.

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
