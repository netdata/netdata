<!--
title: "Collectors configuration reference"
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/REFERENCE.md"
sidebar_label: "Collectors configuration"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Configuration"
-->

# Collectors configuration reference

The list of supported collectors can be found in [the documentation](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md), 
and on [our website](https://www.netdata.cloud/integrations). The documentation of each collector provides all the 
necessary configuration options and prerequisites for that collector. In most cases, either the charts are automatically generated 
without any configuration, or you just fulfil those prerequisites and [configure the collector](#configure-a-collector).

If the application you are interested in monitoring is not listed in our integrations, the collectors list includes 
the available options to 
[add your application to Netdata](https://github.com/netdata/netdata/edit/master/src/collectors/COLLECTORS.md#add-your-application-to-netdata).

If we do support your collector but the charts described in the documentation don't appear on your dashboard, the reason will 
be one of the following:

-   The entire data collection plugin is disabled by default. Read how to [enable and disable plugins](#enable-and-disable-plugins)

-   The data collection plugin is enabled, but a specific data collection module is disabled. Read how to
    [enable and disable a specific collection module](#enable-and-disable-a-specific-collection-module). 

-   Autodetection failed. Read how to [configure](#configure-a-collector) and [troubleshoot](#troubleshoot-a-collector) a collector.

## Enable and disable plugins

You can enable or disable individual plugins by opening `netdata.conf` and scrolling down to the `[plugins]` section.
This section features a list of Netdata's plugins, with a boolean setting to enable or disable them. The exception is
`statsd.plugin`, which has its own `[statsd]` section. Your `[plugins]` section should look similar to this:

```conf
[plugins]
	# timex = yes
	# idlejitter = yes
	# netdata monitoring = yes
	# tc = yes
	# diskspace = yes
	# proc = yes
	# cgroups = yes
	# enable running new plugins = yes
	# check for new plugins every = 60
	# slabinfo = no
	# python.d = yes
	# perf = yes
	# ioping = yes
	# fping = yes
	# nfacct = yes
	# go.d = yes
	# apps = yes
	# ebpf = yes
	# charts.d = yes
	# statsd = yes
```

By default, most plugins are enabled, so you don't need to enable them explicitly to use their collectors. To enable or
disable any specific plugin, remove the comment (`#`) and change the boolean setting to `yes` or `no`.

## Enable and disable a specific collection module

You can enable/disable of the collection modules supported by `go.d`, `python.d` or `charts.d` individually, using the 
configuration file of that orchestrator. For example, you can change the behavior of the Go orchestrator, or any of its 
collectors, by editing `go.d.conf`.

Use `edit-config` from your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md#the-netdata-config-directory) 
to open the orchestrator primary configuration file:

```bash
cd /etc/netdata
sudo ./edit-config go.d.conf
```

Within this file, you can either disable the orchestrator entirely (`enabled: yes`), or find a specific collector and
enable/disable it with `yes` and `no` settings. Uncomment any line you change to ensure the Netdata daemon reads it on
start.

After you make your changes, restart the Agent with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for your system.

## Configure a collector

Most collector modules come with **auto-detection**, configured to work out-of-the-box on popular operating systems with
the default settings. 

However, there are cases that auto-detection fails. Usually, the reason is that the applications to be monitored do not
allow Netdata to connect. In most of the cases, allowing the user `netdata` from `localhost` to connect and collect
metrics, will automatically enable data collection for the application in question (it will require a Netdata restart).

When Netdata starts up, each collector searches for exposed metrics on the default endpoint established by that service
or application's standard installation procedure. For example, 
the [Nginx collector](https://github.com/netdata/netdata/blob/master/src/go/collectors/go.d.plugin/modules/nginx/README.md) searches at
`http://127.0.0.1/stub_status` for exposed metrics in the correct format. If an Nginx web server is running and exposes
metrics on that endpoint, the collector begins gathering them.

However, not every node or infrastructure uses standard ports, paths, files, or naming conventions. You may need to
enable or configure a collector to gather all available metrics from your systems, containers, or applications.

First, [find the collector](https://github.com/netdata/netdata/blob/master/src/collectors/COLLECTORS.md) you want to edit 
and open its documentation. Some software has collectors written in multiple languages. In these cases, you should always 
pick the collector written in Go.

Use `edit-config` from your 
[Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/netdata-agent/configuration/README.md#the-netdata-config-directory) 
to open a collector's configuration file. For example, edit the Nginx collector with the following:

```bash
./edit-config go.d/nginx.conf
```

Each configuration file describes every available option and offers examples to help you tweak Netdata's settings
according to your needs. In addition, every collector's documentation shows the exact command you need to run to
configure that collector. Uncomment any line you change to ensure the collector's orchestrator or the Netdata daemon
read it on start.

After you make your changes, restart the Agent with `sudo systemctl restart netdata`, or the [appropriate
method](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) for your system.

## Troubleshoot a collector

First, navigate to your plugins directory, which is usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case
on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the plugins directory,
switch to the `netdata` user.

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

The output from the relevant command will provide valuable troubleshooting information. If you can't figure out how to
enable the collector using the details from this output, feel free to [join our Discord server](https://discord.com/invite/2mEmfW735j), 
to get help from our experts.
