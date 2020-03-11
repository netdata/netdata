<!--
---
title: "Collectors configuration reference"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/REFERENCE.md
---
-->

# Collectors configuration reference

Welcome to the collector configuration reference guide.

This guide contains detailed information about enabling/disabling plugins or modules, in addition a quick reference to
the internal plugins API.

To learn the basics of collecting metrics from other applications and services, see the [collector
quickstart](QUICKSTART.md).

## What's in this reference guide

-   [Netdata's collector architecture](#netdatas-collector-architecture)
-   [Enable, configure, and disable modules](#enable-configure-and-disable-modules)
-   [Troubleshoot a collector](#troubleshoot-a-collector)
-   [Enable and disable plugins](#enable-and-disable-plugins)
-   [Internal plugins](#internal-plugins)
    -   [Internal plugins API](#internal-plugins-api)
-   [External plugins](#external-plugins)
-   [Write a custom collector](#write-a-custom-collector)

## Netdata's collector architecture

Netdata has an intricate system for organizing and managing its collectors. **Collectors** are the processes/programs
that actually gather metrics from various sources. Collectors are organized by **plugins**, which help manage all the
independent processes in a variety of programming languages based on their purpose and performance requirements.
**Modules** are a type of collector, used primarily to connect to external applications, such as an Nginx web server or
MySQL database, among many others.

For most users, enabling individual collectors for the application/service you're interested in is far more important
than knowing which plugin it uses. See our [collectors list](COLLECTORS.md) to see whether your favorite app/service has
a collector, and then read the [collectors quickstart](QUICKSTART.md) and the documentation for that specific collector
to figure out how to enable it.

There are three types of plugins:

-   **Internal** plugins organize collectors that gather metrics from `/proc`, `/sys` and other Linux kernel sources.
    They are written in `C`, and run as threads within the Netdata daemon.
-   **External** plugins organize collectors that gather metrics from external processes, such as a MySQL database or
    Nginx web server. They can be written in any language, and the `netdata` daemon spawns them as long-running
    independent processes. They communicate with the daemon via pipes.
-   **Plugin orchestrators**, which are external plugins that instead support a number of **modules**. Modules are a
    type of collector. We have a few plugin orchestrators available for those who want to develop their own collectors,
    but focus most of our efforts on the [Go plugin](go.d.plugin/README.md).

## Enable, configure, and disable modules

Most collector modules come with **auto-detection**, configured to work out-of-the-box on popular operating systems with
the default settings.

However, there are cases that auto-detection fails. Usually, the reason is that the applications to be monitored do not
allow Netdata to connect. In most of the cases, allowing the user `netdata` from `localhost` to connect and collect
metrics, will automatically enable data collection for the application in question (it will require a Netdata restart).

View our [collectors quickstart](QUICKSTART.md) for explict details on enabling and configuring collector modules.

## Troubleshoot a collector

First, navigate to your plugins directory, which is usually at `/usr/libexec/netdata/plugins.d/`. If that's not the case
on your system, open `netdata.conf` and look for the setting `plugins directory`. Once you're in the plugins directory,
switch to the `netdata` user.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash netdata
```

The next step is based on the collector's orchestrator. You can figure out which orchestrator the collector uses by 

uses either
by viewing the [collectors list](COLLECTORS.md) and referencing the _configuration file_ field. For example, if that
field contains `go.d`, that collector uses the Go orchestrator.

```bash
# Go orchestrator (go.d.plugin)
./go.d.plugin -d -m <MODULE_NAME>

# Python orchestrator (python.d.plugin)
./python.d.plugin <MODULE_NAME> debug trace

# Node orchestrator (node.d.plugin)
./node.d.plugin debug 1 <MODULE_NAME>

# Bash orchestrator (bash.d.plugin)
./charts.d.plugin debug 1 <MODULE_NAME>
```

The output from the relevant command will provide valuable troubleshooting information. If you can't figure out how to
enable the collector using the details from this output, feel free to [create an issue on our
GitHub](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md) to get some
help from our collectors experts.

## Enable and disable plugins

You can enable or disable individual plugins by opening `netdata.conf` and scrolling down to the `[plugins]` section.
This section features a list of Netdata's plugins, with a boolean setting to enable or disable them. The exception is
`statsd.plugin`, which has its own `[statsd]` section. Your `[plugins]` section should look similar to this:

```conf
[plugins]
	# PATH environment variable = /usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/var/lib/snapd/snap/bin:/sbin:/usr/sbin:/usr/local/bin:/usr/local/sbin
	# PYTHONPATH environment variable = 
	# proc = yes
	# diskspace = yes
	# cgroups = yes
	# tc = yes
	# idlejitter = yes
	# enable running new plugins = yes
	# check for new plugins every = 60
	# slabinfo = no
	# fping = yes
	# ioping = yes
	# node.d = yes
	# python.d = yes
	# go.d = yes
	# apps = yes
	# perf = yes
	# charts.d = yes
```

By default, most plugins are enabled, so you don't need to enable them explicity to use their collectors. To enable or
disable any specific plugin, remove the comment (`#`) and change the boolean setting to `yes` or `no`.

All **external plugins** are managed by [plugins.d](plugins.d/), which provides additional management options.

## Internal plugins

Each of the internal plugins runs as a thread inside the `netdata` daemon. Once this thread has started, the plugin may
spawn additional threads according to its design.

### Internal plugins API

The internal data collection API consists of the following calls:

```c
collect_data() {
    // collect data here (one iteration)

    collected_number collected_value = collect_a_value();

    // give the metrics to Netdata

    static RRDSET *st = NULL; // the chart
    static RRDDIM *rd = NULL; // a dimension attached to this chart

    if(unlikely(!st)) {
        // we haven't created this chart before
        // create it now
        st = rrdset_create_localhost(
                "type"
                , "id"
                , "name"
                , "family"
                , "context"
                , "Chart Title"
                , "units"
                , "plugin-name"
                , "module-name"
                , priority
                , update_every
                , chart_type
        );

        // attach a metric to it
        rd = rrddim_add(st, "id", "name", multiplier, divider, algorithm);
    }
    else {
        // this chart is already created
        // let Netdata know we start a new iteration on it
        rrdset_next(st);
    }

    // give the collected value(s) to the chart
    rrddim_set_by_pointer(st, rd, collected_value);

    // signal Netdata we are done with this iteration
    rrdset_done(st);
}
```

Of course, Netdata has a lot of libraries to help you also in collecting the metrics. The best way to find your way
through this, is to examine what other similar plugins do.

## External Plugins

**External plugins** use the API and are managed by [plugins.d](plugins.d/).

## Write a custom collector

You can add custom collectors by following the [external plugins documentation](../collectors/plugins.d/).
