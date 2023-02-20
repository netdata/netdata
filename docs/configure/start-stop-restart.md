<!--
title: "Start, stop, or restart the Netdata Agent"
description: "Manage the Netdata Agent daemon, load configuration changes, and troubleshoot stuck processes on systemd and non-systemd nodes."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/docs/configure/start-stop-restart.md"
sidebar_label: "Start, stop, or restart the Netdata Agent"
learn_status: "Published"
learn_topic_type: "Tasks"
learn_rel_path: "Operations"
-->

# Start, stop, or restart the Netdata Agent

When you install the Netdata Agent, the [daemon](https://github.com/netdata/netdata/blob/master/daemon/README.md) is configured to start at boot and stop and
restart/shutdown.

You will most often need to _restart_ the Agent to load new or editing configuration files. [Health
configuration](#reload-health-configuration) files are the only exception, as they can be reloaded without restarting
the entire Agent.

Stopping or restarting the Netdata Agent will cause gaps in stored metrics until the `netdata` process initiates
collectors and the database engine.

## Using `systemctl`, `service`, or `init.d`

This is the recommended way to start, stop, or restart the Netdata daemon.

- To **start** Netdata, run `sudo systemctl start netdata`.
- To **stop** Netdata, run `sudo systemctl stop netdata`.
- To **restart** Netdata, run `sudo systemctl restart netdata`.

If the above commands fail, or you know that you're using a non-systemd system, try using the `service` command:

- **service**: `sudo service netdata start`, `sudo service netdata stop`, `sudo service netdata restart`

## Using `netdata`

Use the `netdata` command, typically located at `/usr/sbin/netdata`, to start the Netdata daemon. 

```bash
sudo netdata
```

If you start the daemon this way, close it with `sudo killall netdata`.

## Using `netdatacli`

The Netdata Agent also comes with a [CLI tool](https://github.com/netdata/netdata/blob/master/cli/README.md) capable of performing shutdowns. Start the Agent back up
using your preferred method listed above.

```bash
sudo netdatacli shutdown-agent
```

## Reload health configuration

You do not need to restart the Netdata Agent between changes to health configuration files, such as specific health
entities. Instead, use [`netdatacli`](#using-netdatacli) and the `reload-health` option to prevent gaps in metrics
collection.

```bash
sudo netdatacli reload-health
```

If `netdatacli` doesn't work on your system, send a `SIGUSR2` signal to the daemon, which reloads health configuration
without restarting the entire process.

```bash
killall -USR2 netdata
```

## Force stop stalled or unresponsive `netdata` processes

In rare cases, the Netdata Agent may stall or not properly close sockets, preventing a new process from starting. In
these cases, try the following three commands:

```bash
sudo systemctl stop netdata
sudo killall netdata
ps aux| grep netdata
```

The output of `ps aux` should show no `netdata` or associated processes running. You can now start the Netdata Agent
again with `service netdata start`, or the appropriate method for your system.
