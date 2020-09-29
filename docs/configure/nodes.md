<!--
title: "Configure your nodes"
description: "Netdata is zero-configuration for most users, but complex infrastructures may require you to tweak some of the Agent's granular settings."
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/configure/nodes.md
-->

# Configure your nodes

Netdata's zero-configuration collection, storage, and visualization features work for many users, infrastructures, and
use cases, but there are some situations where you might want to configure your nodes.

For example, you might want to increase metrics retention, configure a collector based on your infrastructure's unique
setup, or secure the local dashboard by restricting it to only connections from `localhost`.

Whatever the reason, Netdata users should know how to configure individual nodes to act decisively if an incident,
anomaly, or change in infrastructure affects how their Agents should peform.

## The Netdata config directory

On most Linux systems, using our [recommended one-line installation](/docs/get/README.md#install-the-netdata-agent), the
**Netdata config directory** is `/etc/netdata/`. The config directory contains several configuration files with the
`.conf` extension, a few directories, and a shell script named `edit-config`.

> Some operating systems will use `/opt/netdata/etc/netdata/` as the config directory. If you're not sure where yours
> is, navigate to `http://NODE:19999/netdata.conf` in your browser, replacing `NODE` with the IP address or hostname of
> your node, and find the `# config directory = ` setting. The value listed is the config directory for your system.

All of Netdata's documentation assumes that your config directory is at `/etc/netdata`, and that you're running any
scripts from inside that directory.

## Netdata's configuration files

Upon installation, the Netdata config directory contains a few files and directories.

-   `netdata.conf` is the main configuration file. This is where you'll find most configuration options. This doc won't
    go into exhaustive detail about each setting. You can read descriptions for each in the [daemon config
    doc](/daemon/config/README.md).
-   `orig` is a symbolic link to the directory `/usr/lib/netdata/conf.d`, which contains stock configuration files.
    Stock versions are copied into the config directory when opened with `edit-config`. _Do not edit the files in
    `/usr/lib/netdata/conf.d`, as they are overwritten by updates to the Netdata Agent._
-   `edit-config` is a shell script used for [editing configuration files](#use-edit-config-to-edit-netdataconf).
-   `go.d/`, `python.d/`, `charts.d/`, `node.d`/, and `custom-plugins.d/`, which are directories for each of Netdata's
    [orchestrators](/collectors/plugins.d/README.md#external-plugins-overview). These directories can each contain
    additional `.conf` files for configuring specific collectors.

## Use `edit-config` to edit `netdata.conf`

The best way to edit any configuration file is with `edit-config` script. This script opens existing Netdata
configuration files using your system's `$EDITOR`. If the file doesn't yet exist in your config directory, the script
copies the stock version from `/usr/lib/netdata/conf.d` and opens it for editing.

`edit-config` is the recommended way to easily and safely edit Netdata's configuration.

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
> defaulted to `vim` or `nano`. To change this variable for the current session (it will revert to the default when you
> reboot), export a new value: `export EDITOR=nano`. Or, [make the change
> permanent](https://stackoverflow.com/questions/13046624/how-to-permanently-export-a-variable-in-linux).

After you make your changes, you need to restart the Agent with `service netdata restart`.

Here's an example of editing the node's hostname, which appears in both the local dashboard and in Netdata Cloud.

![Animated GIF of editing the hostname option in
netdata.conf](https://user-images.githubusercontent.com/1153921/80994808-1c065300-8df2-11ea-81af-d28dc3ba27c8.gif)

### Other configuration files

You can edit any Netdata configuration file using `edit-config`. A few examples:

```bash
./edit-config apps_groups.conf
./edit-config ebpf.conf
./edit-config health.d/load.conf
./edit-config go.d/prometheus.conf
```

The documentation for each of Netdata's components explains which file(s) to edit to achieve the desired behavior.

## What's next?

Take advantage of this newfound understanding of node configuration to [add security to your
node](/docs/configure/secure-nodes.md). We have a few best practices based on how you use the Netdata Agent and Netdata
Cloud.

You can also take what you've learned about node configuration to tweak the Agent's behavior or enable new features:

-   [Enable new collectors](/docs/collect/enable-configure.md) or tweak their behavior.
-   [Configure existing health alarms](/docs/monitor/configure-alarms.md) or create new ones.
-   [Enable notifications](/docs/monitor/enable-notifications.md) to receive updates about the health of your
    infrastructure.
-   Change [the long-term metrics retention period](/docs/store/change-metrics-storage.md) using the database engine.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fconfigure%2Fnodesa&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
