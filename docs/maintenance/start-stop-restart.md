# Start, stop, or restart the Netdata Agent

When you install the Netdata Agent, the [daemon](https://github.com/netdata/netdata/blob/master/src/daemon/README.md) is 
configured to start at boot and stop and restart/shutdown.

You will most often need to _restart_ the Agent to load new or editing configuration files. 
[Health configuration](#reload-health-configuration) files are the only exception, as they can be reloaded without restarting
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

The Netdata Agent also comes with a [CLI tool](https://github.com/netdata/netdata/blob/master/src/cli/README.md) capable of performing shutdowns. Start the Agent back up
using your preferred method listed above.

```bash
sudo netdatacli shutdown-agent
```

## Netdata MSI installations

Netdata provides an installer for Windows using WSL, on those installations by using a Windows terminal (e.g. the Command prompt or Windows Powershell) you can:

- Start Netdata, by running `start-netdata`
- Stop Netdata, by running `stop-netdata`
- Restart Netdata, by running `restart-netdata`

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

## Starting Netdata at boot

In the `system` directory you can find scripts and configurations for the
various distros.

### systemd

The installer already installs `netdata.service` if it detects a systemd system.

To install `netdata.service` by hand, run:

```sh
# stop Netdata
killall netdata

# copy netdata.service to systemd
cp system/netdata.service /etc/systemd/system/

# let systemd know there is a new service
systemctl daemon-reload

# enable Netdata at boot
systemctl enable netdata

# start Netdata
systemctl start netdata
```

### init.d

In the system directory you can find `netdata-lsb`. Copy it to the proper place according to your distribution
documentation. For Ubuntu, this can be done via running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-lsb /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
update-rc.d netdata defaults
```

### openrc (gentoo)

In the `system` directory you can find `netdata-openrc`. Copy it to the proper
place according to your distribution documentation.

### CentOS / Red Hat Enterprise Linux

For older versions of RHEL/CentOS that don't have systemd, an init script is included in the system directory. This can
be installed by running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-init-d /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
chkconfig --add netdata
```

_There have been some recent work on the init script, see PR
<https://github.com/netdata/netdata/pull/403>_

### other systems

You can start Netdata by running it from `/etc/rc.local` or equivalent.
