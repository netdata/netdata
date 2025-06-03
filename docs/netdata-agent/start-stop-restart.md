# Service Control

:::tip

**What You'll Learn**

How to start, stop, restart, and reload the Netdata Agent across different operating systems and service management systems.

:::

The Netdata Agent automatically starts at boot after installation.

:::important

**Important**

In most cases, you need to **restart the Netdata service** to apply changes to configuration files. Health configuration files, which define alerts, are an exception. They can be [reloaded](#reload-health) **without restarting**.

:::

:::warning

**Service Restart Impact**

Restarting the Netdata Agent will cause temporary gaps in your collected metrics. This occurs while the netdata process reinitializes its data collectors and database engine.

:::

## UNIX

### Using `systemctl`, `service`, or `init.d`

| Action  | Systemd                          | Non-systemd                    |
|---------|----------------------------------|--------------------------------|
| start   | `sudo systemctl start netdata`   | `sudo service netdata start`   |
| stop    | `sudo systemctl stop netdata`    | `sudo service netdata stop`    |
| restart | `sudo systemctl restart netdata` | `sudo service netdata restart` |

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

:::note

**Administrator Access Required**

You will need to run PowerShell as administrator.

:::

### Using Windows Services GUI

<details>
<summary><strong>Manage Through Task Manager</strong></summary><br/>

If you prefer to manage the Agent through the GUI, you can start-stop and restart the `Netdata` service from the "Services" tab of Task Manager.

<br/>
</details>

### Using PowerShell Commands

- To **start** Netdata, run `Start-Service Netdata`.
- To **stop** Netdata, run `Stop-Service Netdata`.
- To **restart** Netdata, run `Restart-Service Netdata`.

## Quick Reference

### UNIX Commands Summary

| Task              | Systemd                          | Non-systemd                      | Direct Command                   |
|-------------------|----------------------------------|----------------------------------|----------------------------------|
| **Start**         | `sudo systemctl start netdata`   | `sudo service netdata start`     | `sudo netdata`                   |
| **Stop**          | `sudo systemctl stop netdata`    | `sudo service netdata stop`      | `sudo killall netdata`           |
| **Restart**       | `sudo systemctl restart netdata` | `sudo service netdata restart`   | Stop + Start                     |
| **Reload Health** | `sudo netdatacli reload-health`  | `sudo netdatacli reload-health`  | `sudo netdatacli reload-health`  |
| **Shutdown**      | `sudo netdatacli shutdown-agent` | `sudo netdatacli shutdown-agent` | `sudo netdatacli shutdown-agent` |

### Windows Commands Summary

| Task        | PowerShell Command        | GUI Location                      |
|-------------|---------------------------|-----------------------------------|
| **Start**   | `Start-Service Netdata`   | Task Manager > Services > Netdata |
| **Stop**    | `Stop-Service Netdata`    | Task Manager > Services > Netdata |
| **Restart** | `Restart-Service Netdata` | Task Manager > Services > Netdata |
