<!--
title: "Configure the Netdata Agent"
description: "Netdata is zero-configuration for most users, but complex infrastructures may require you to tweak some of the Agent's granular settings."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/nodes.md
-->

# Configure the Netdata Agent

Netdata's zero-configuration collection, storage, and visualization features work for many users, infrastructures, and
use cases, but there are some situations where you might want to configure the Netdata Agent running on your node(s),
which can be a physical or virtual machine (VM), container, cloud deployment, or edge/IoT device.

For example, you might want to increase metrics retention, configure a collector based on your infrastructure's unique
setup, or secure the local dashboard by restricting it to only connections from `localhost`.

Whatever the reason, Netdata users should know how to configure individual nodes to act decisively if an incident,
anomaly, or change in infrastructure affects how their Agents should perform.

## The Netdata config directory

On most Linux systems, using our [recommended one-line
installation](/docs/get-started.mdx#install-on-linux-with-one-line-installer), the **Netdata config
directory** is `/etc/netdata/`. The config directory contains several configuration files with the `.conf` extension, a
few directories, and a shell script named `edit-config`.

> Some operating systems will use `/opt/netdata/etc/netdata/` as the config directory. If you're not sure where yours
> is, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with the IP address or hostname of
> your node, and find the `# config directory = ` setting. The value listed is the config directory for your system.

All of Netdata's documentation assumes that your config directory is at `/etc/netdata`, and that you're running any
scripts from inside that directory.

## Netdata's configuration files

Upon installation, the Netdata config directory contains a few files and directories. It's okay if you don't see all
these files in your own Netdata config directory, as the next section describes how to edit any that might not already
exist.

- `netdata.conf` is the main configuration file. This is where you'll find most configuration options. Read descriptions
  for each in the [daemon config](/daemon/config/README.md) doc.
- `edit-config` is a shell script used for [editing configuration files](#use-edit-config-to-edit-configuration-files).
- Various configuration files ending in `.conf` for [configuring plugins or
  collectors](/docs/collect/enable-configure.md#enable-a-collector-or-its-orchestrator) behave. Examples: `go.d.conf`,
  `python.d.conf`, and `ebpf.d.conf`.
- Various directories ending in `.d`, which contain other configuration files, each ending in `.conf`, for [configuring
  specific collectors](/docs/collect/enable-configure.md#configure-a-collector).
- `apps_groups.conf` is a configuration file for changing how applications/processes are grouped when viewing the
  **Application** charts from [`apps.plugin`](/collectors/apps.plugin/README.md) or
  [`ebpf.plugin`](/collectors/ebpf.plugin/README.md).
- `health.d/` is a directory that contains [health configuration files](/docs/monitor/configure-alarms.md).
- `health_alarm_notify.conf` enables and configures [alarm notifications](/docs/monitor/enable-notifications.md).
- `statsd.d/` is a directory for configuring Netdata's [statsd collector](/collectors/statsd.plugin/README.md).
- `stream.conf` configures [parent-child streaming](/streaming/README.md) between separate nodes running the Agent.
- `.environment` is a hidden file that describes the environment in which the Netdata Agent is installed, including the
  `PATH` and any installation options. Useful for [reinstalling](/packaging/installer/REINSTALL.md) or
  [uninstalling](/packaging/installer/UNINSTALL.md) the Agent.

The Netdata config directory also contains one symlink:

- `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains stock configuration files. Stock
  versions are copied into the config directory when opened with `edit-config`. _Do not edit the files in
  `/usr/lib/netdata/conf.d`, as they are overwritten by updates to the Netdata Agent._

## Configure a Netdata docker container

See [configure agent containers](/packaging/docker/README.md#configure-agent-containers).

## Use `edit-config` to edit configuration files

The **recommended way to easily and safely edit Netdata's configuration** is with the `edit-config` script. This script
opens existing Netdata configuration files using your system's `$EDITOR`. If the file doesn't yet exist in your config
directory, the script copies the stock version from `/usr/lib/netdata/conf.d` and opens it for editing.

Run `edit-config` without any options to see details on its usage and a list of all the configuration files you can
edit.

```bash
./edit-config
USAGE:
  ./edit-config FILENAME

  Copy and edit the stock config file named: FILENAME
  if FILENAME is already copied, it will be edited as-is.

  The EDITOR shell variable is used to define the editor to be used.

  Stock config files at: '/usr/lib/netdata/conf.d'
  User  config files at: '/etc/netdata'

  Available files in '/usr/lib/netdata/conf.d' to copy and edit:

./apps_groups.conf                  ./health.d/phpfpm.conf
./aws_kinesis.conf                  ./health.d/pihole.conf
./charts.d/ap.conf                  ./health.d/portcheck.conf
./charts.d/apcupsd.conf             ./health.d/postgres.conf
...
```

To edit `netdata.conf`, run `./edit-config netdata.conf`. You may need to elevate your privileges with `sudo` or another
method for `edit-config` to write into the config directory. Use your `$EDITOR`, make your changes, and save the file.

> `edit-config` uses the `EDITOR` environment variable on your system to edit the file. On many systems, that is
> defaulted to `vim` or `nano`. Use `export EDITOR=` to change this temporarily, or edit your shell configuration file
> to change to permanently.

After you make your changes, you need to [restart the Agent](/docs/configure/start-stop-restart.md) with `sudo systemctl
restart netdata` or the appropriate method for your system.

Here's an example of editing the node's hostname, which appears in both the local dashboard and in Netdata Cloud.

![Animated GIF of editing the hostname option in
netdata.conf](https://user-images.githubusercontent.com/1153921/80994808-1c065300-8df2-11ea-81af-d28dc3ba27c8.gif)

### Other configuration files

You can edit any Netdata configuration file using `edit-config`. A few examples:

```bash
./edit-config apps_groups.conf
./edit-config ebpf.d.conf
./edit-config health.d/load.conf
./edit-config go.d/prometheus.conf
```

The documentation for each of Netdata's components explains which file(s) to edit to achieve the desired behavior.

## See an Agent's running configuration

On start, the Netdata Agent daemon attempts to load `netdata.conf`. If that file is missing, incomplete, or contains
invalid settings, the daemon attempts to run sane defaults instead. In other words, the state of `netdata.conf` on your
filesystem may be different from the state of the Netdata Agent itself.

To see the _running configuration_, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with
the IP address or hostname of your node. The file displayed here is exactly the settings running live in the Netdata
Agent.

If you're having issues with configuring the Agent, apply the running configuration to `netdata.conf` by downloading the
file to the Netdata config directory. Use `sudo` to elevate privileges.

```bash
wget -O /etc/netdata/netdata.conf http://localhost:19999/netdata.conf
# or
curl -o /etc/netdata/netdata.conf http://NODE:19999/netdata.conf
```

## What's next?

Learn more about [starting, stopping, or restarting](/docs/configure/start-stop-restart.md) the Netdata daemon to apply
configuration changes.

Apply some [common configuration changes](/docs/configure/common-changes.md) to quickly tweak the Agent's behavior.

[Add security to your node](/docs/configure/secure-nodes.md) with what you've learned about the Netdata config directory
and `edit-config`. We put together a few security best practices based on how you use the Netdata.

You can also take what you've learned about node configuration to enable or enhance features:

-   [Enable new collectors](/docs/collect/enable-configure.md) or tweak their behavior.
-   [Configure existing health alarms](/docs/monitor/configure-alarms.md) or create new ones.
-   [Enable notifications](/docs/monitor/enable-notifications.md) to receive updates about the health of your
    infrastructure.
-   Change [the long-term metrics retention period](/docs/store/change-metrics-storage.md) using the database engine.

### Related reference documentation

- [Netdata Agent · Daemon](/daemon/README.md)
- [Netdata Agent · Health monitoring](/health/README.md)
- [Netdata Agent · Notifications](/health/notifications/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fnodes&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
