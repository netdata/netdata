# Add more charts to Netdata

Netdata collects system metrics by itself. It has many [internal plugins](../collectors) for collecting most of the metrics presented by default when it starts, collecting data from `/proc`, `/sys` and other Linux kernel sources.



## configuring plugins

Netdata comes with **internal** and **external** plugins:

1.  The **internal** ones are written in `C` and run as threads within the Netdata daemon.
2.  The **external** ones can be written in any computer language. The Netdata daemon spawns these as processes (shown with `ps fax`) and reads their metrics using pipes (so the `stdout` of external plugins is connected to Netdata for metrics collection and the `stderr` of external plugins is connected to `/var/log/netdata/error.log`).

To make it easier to develop plugins, and minimize the number of threads and processes running, Netdata supports **plugin orchestrators**, each of them supporting one or more data collection **modules**. Currently we ship plugin orchestrators for 4 languages: `C`, `python`, `node.js` and `bash` and 2 more are under development (`go` and `java`).

#### enabling and disabling plugins

To control which plugins Netdata run, edit `netdata.conf` and check the `[plugins]` section. It looks like this:

```
[plugins]
	# enable running new plugins = yes
	# check for new plugins every = 60
	# proc = yes
	# diskspace = yes
	# cgroups = yes
	# cups = yes
	# tc = yes
	# nfacct = yes
	# idlejitter = yes
	# freeipmi = yes
	# node.d = yes
	# python.d = yes
	# fping = yes
	# ioping = yes
	# charts.d = yes
	# apps = yes
	# xenstat = yes
	# perf = no
	# slabinfo = no
```

The default for all plugins is the option `enable running new plugins`. So, setting this to `no` will disable all the plugins, except the ones specifically enabled.

#### enabling and disabling modules

Each of the **plugins** may support one or more data collection **modules**.  To control which of its modules run, you have to consult the configuration of the **plugin** (see table below).

#### modules configuration

Most **modules** come with **auto-detection**, configured to work out-of-the-box on popular operating systems with the default settings.

However, there are cases that auto-detection fails. Usually the reason is that the applications to be monitored do not allow Netdata to connect. In most of the cases, allowing the user `netdata` from `localhost` to connect and collect metrics, will automatically enable data collection for the application in question (it will require a Netdata restart).

You can verify Netdata **external plugins and their modules** are able to collect metrics, following this procedure:

```sh
# become user netdata
sudo su -s /bin/bash netdata

# execute the plugin in debug mode, for a specific module.
# example for the python plugin, mysql module:
/usr/libexec/netdata/plugins.d/python.d.plugin 1 debug trace mysql
```

Similarly, you can use `charts.d.plugin` for BASH plugins and `node.d.plugin` for node.js plugins.
Other plugins (like `apps.plugin`, `freeipmi.plugin`, `fping.plugin`, `ioping.plugin`, `nfacct.plugin`, `xenstat.plugin`, `perf.plugin`, `slabinfo.plugin`) use the native Netdata plugin API and can be run directly.

If you need to configure a Netdata plugin or module, all user supplied configuration is kept at `/etc/netdata` while the stock versions of all files is at `/usr/lib/netdata/conf.d`.
To copy a stock file and edit it, run `/etc/netdata/edit-config`. Running this command without an argument, will list the available stock files.

Each file should provide plenty of examples and documentation about each module and plugin.

This is a map of the all supported configuration options:

#### map of configuration files

| plugin | language | plugin<br/>configuration | modules<br/>configuration |
|-----:|:------:|:----------------------:|:------------------------|
| `apps.plugin`<br/>(external plugin for monitoring the process tree on Linux and FreeBSD)|`C`|`netdata.conf` section `[plugin:apps]`|Custom configuration for the processes to be monitored at `apps_groups.conf`|
| `freebsd.plugin`<br/>(internal plugin for monitoring FreeBSD system resources)|`C`|`netdata.conf` section `[plugin:freebsd]`|one section for each module `[plugin:freebsd:MODULE]`. Each module may provide additional sections in the form of `[plugin:freebsd:MODULE:SUBSECTION]`.|
| `cgroups.plugin`<br/>(internal plugin for monitoring Linux containers, VMs and systemd services)|`C`|`netdata.conf` section `[plugin:cgroups]`|N/A|
| `charts.d.plugin`<br/>(external plugin orchestrator for BASH modules)|`BASH`|`charts.d.conf`|a file for each module in `/etc/netdata/charts.d/`|
| `diskspace.plugin`<br/>(internal plugin for collecting Linux mount points usage)|`C`|`netdata.conf` section `[plugin:diskspace]`|N/A|
| `fping.plugin`<br/>(external plugin for collecting network latencies)|`C`|`fping.conf`|This plugin is a wrapper for the `fping` command.|
| `ioping.plugin`<br/>(external plugin for collecting disk latencies)|`C`|`ioping.conf`|This plugin is a wrapper for the `ioping` command.|
| `freeipmi.plugin`<br/>(external plugin for collecting IPMI h/w sensors)|`C`|`netdata.conf` section `[plugin:freeipmi]`||
| `nfacct.plugin`<br/>(external plugin for monitoring netfilter firewall and connection tracker)|`C`|`netdata.conf` section `[plugin:nfacct]`|N/A|
| `xenstat.plugin`<br/>(external plugin for monitoring XCP-ng and XenServer)|`C`|`netdata.conf` section `[plugin:xenstat]`|N/A|
| `perf.plugin`<br/>(external plugin for monitoring CPU performance on Linux)|`C`|`netdata.conf` section `[plugin:perf]`|N/A|
| `idlejitter.plugin`<br/>(internal plugin for monitoring CPU jitter)|`C`|N/A|N/A|
| `macos.plugin`<br/>(internal plugin for monitoring MacOS system resources)|`C`|`netdata.conf` section `[plugin:macos]`|one section for each module `[plugin:macos:MODULE]`. Each module may provide additional sections in the form of `[plugin:macos:MODULE:SUBSECTION]`.|
| `node.d.plugin`<br/>(external plugin orchestrator of node.js modules)|`node.js`|`node.d.conf`|a file for each module in `/etc/netdata/node.d/`.|
| `proc.plugin`<br/>(internal plugin for monitoring Linux system resources)|`C`|`netdata.conf` section `[plugin:proc]`|one section for each module `[plugin:proc:MODULE]`. Each module may provide additional sections in the form of `[plugin:proc:MODULE:SUBSECTION]`.|
| `python.d.plugin`<br/>(external plugin orchestrator for running python modules)|`python`<br/>v2 or v3<br/>both are supported|`python.d.conf`|a file for each module in `/etc/netdata/python.d/`.|
| `statsd.plugin`<br/>(internal plugin for collecting statsd metrics)|`C`|`netdata.conf` section `[statsd]`|Synthetic statsd charts can be configured with files in `/etc/netdata/statsd.d/`.|
| `slabinfo.plugin`<br/>(external plugin for monitoring Kernel SLAB cache on Linux)|`C`|`netdata.conf` section `[plugin:slabinfo]`|N/A|
| `tc.plugin`<br/>(internal plugin for collecting Linux traffic QoS)|`C`|`netdata.conf` section `[plugin:tc]`|The plugin runs an external helper called `tc-qos-helper.sh` to interface with the `tc` command. This helper supports a few additional options using `tc-qos-helper.conf`.|

## writing data collection modules

You can add custom plugins following the [External Plugins Guide](../collectors/plugins.d/).

---



[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2FAdd-more-charts-to-netdata&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
