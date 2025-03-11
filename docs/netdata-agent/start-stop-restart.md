# Service Control

The Netdata Agent automatically starts at boot after installation.

> In most cases, you need to **restart the Netdata service** to apply changes to configuration files. Health configuration files, which define alerts, are an exception. They can be [reloaded](#reload-health) **without restarting**.
>
> Restarting the Netdata Agent will cause temporary gaps in your collected metrics. This occurs while the netdata process reinitializes its data collectors and database engine.

## UNIX

### Using `systemctl`, `service`, or `init.d`

| Action  | Systemd                         | Non-systemd                    |
|---------|---------------------------------|--------------------------------|
| start   | `sudo systemctl start netdata`  | `sudo service netdata start`   |
| stop    | `sudo systemctl stop netdata`   | `sudo service netdata stop`    |
| restart | `sudo systemctl restart netdata`| `sudo service netdata restart` |

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

### Reload health

No need to restart the Netdata Agent after modifying health configuration files (alerts). Use `netdatacli` to avoid metric collection gaps.

```bash
sudo netdatacli reload-health
```

## Windows

> **Note**
>
> You will need to run PowerShell as administrator.

- To **start** Netdata, run `Start-Service Netdata`.
- To **stop** Netdata, run `Stop-Service Netdata`.
- To **restart** Netdata, run `Restart-Service Netdata`.

If you prefer to manage the Agent through the GUI, you can start-stop and restart the `Netdata` service from the "Services" tab of Task Manager.
