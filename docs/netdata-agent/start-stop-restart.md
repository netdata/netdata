# Start, stop, or restart the Netdata Agent

When you install the Netdata Agent, the [daemon](/src/daemon/README.md) is configured to start at boot and stop and restart/shutdown.

> You will most often need to _restart_ the Agent to load new or editing configuration files. [Health configuration](/docs/netdata-agent/reload-health-configuration.md) files are the only exception, as they can be reloaded without restarting the entire Agent.
>
> Stopping or restarting the Netdata Agent will cause gaps in stored metrics until the `netdata` process initiates collectors and the database engine.

## Unix systems

### Using `systemctl`, `service`, or `init.d`

This is the recommended way to start, stop, or restart the Netdata daemon.

- To **start** Netdata, run `sudo systemctl start netdata`.
- To **stop** Netdata, run `sudo systemctl stop netdata`.
- To **restart** Netdata, run `sudo systemctl restart netdata`.

If the above commands fail, or you know that you're using a non-systemd system, try using the `service` command:

- **service**: `sudo service netdata start`, `sudo service netdata stop`, `sudo service netdata restart`

### Using `netdata`

Use the `netdata` command, typically located at `/usr/sbin/netdata`, to start the Netdata daemon.

```bash
sudo netdata
```

If you start the daemon this way, close it with `sudo killall netdata`.

### Using `netdatacli`

The Netdata Agent also comes with a [CLI tool](/src/cli/README.md) capable of performing shutdowns. Start the Agent back up using your preferred method listed above.

```bash
sudo netdatacli shutdown-agent
```

## Windows systems

> **Note**
>
> You will need to run Powershell as administrator.

- To **start** Netdata, run `Start-Service Netdata`.
- To **stop** Netdata, run `Stop-Service Netdata`.
- To **restart** Netdata, run `Restart-Service Netdata`.
