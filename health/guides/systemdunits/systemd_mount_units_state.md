# systemd_mount_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the `systemd.mount` units state. The `systemd_mount_units_state` alert
indicates that one or more of the `systemd.mount` units are in the failed state.
A systemd mount unit "failed" when the service process returned error code on exit, or crashed, an 
operation timed out, or after too many restarts. The cause of a failed states is stored in a log.

<details>
<summary>Read more about systemd</summary>

Here is some useful information about systemd from
wikipedia <sup>[1](https://en.wikipedia.org/wiki/Systemd) </sup>

Systemd includes features like on-demand starting of daemons, snapshot support, process tracking,
and Inhibitor Locks. Systemd is not just the name of the `init` daemon, but also refers to the
entire software bundle around it, which, in addition to the `systemd` `init` daemon, includes the
daemons
`journald`, `logind` and `networkd`, and many other low-level components. In January 2013,
Poettering described systemd not as one program, but rather a large software suite that includes 69
individual binaries. As an integrated software suite, systemd replaces the startup sequences and
runlevels controlled by the traditional `init` daemon, along with the shell scripts executed under
its control. systemd also integrates many other services that are common on Linux systems by
handling user logins, the system console, device hotplugging, scheduled execution (replacing cron),
logging, hostnames and locales.

Like the `init` daemon, `systemd` is a daemon that manages other daemons, which, including `systemd`
itself, are background processes. `systemd` is the first daemon to start during booting and the last
daemon to terminate during shutdown. The `systemd` daemon serves as the root of the user space's
process tree. The first process (`PID1`) has a special role on Unix systems, as it replaces the
parent of a process when the original parent terminates. Therefore, the first process is
particularly well suited for the purpose of monitoring daemons.

Systemd executes elements of its startup sequence in parallel, which is theoretically faster than
the traditional startup sequence approach. For inter-process communication (IPC), `systemd` makes
Unix domain sockets and D-Bus available to the running daemons. The state of systemd itself can also
be preserved in a snapshot for future recall.

Systemd's core components include the following:

- `systemd` is a system and service manager for Linux operating systems.

- `systemctl` is a command to introspect and control the state of the systemd system and service
  manager. Not to be confused with sysctl.

- `systemd-analyze` may be used to determine system boot-up performance statistics and retrieve
  other state and tracing information from the system and service manager.

</details>



<details>
<summary>See more on systemd-mount units</summary>

A unit configuration file whose name ends in `.mount` encodes information about a file system mount
point controlled and supervised by `systemd`. Additional options are listed in _systemd.exec(5)_,
which define the execution environment the _mount(8)_ program is executed in, and in _systemd.kill(
5)_, which define the way the processes are terminated, and in
_systemd.resource-control(5)_, which configure resource control settings for the processes of the
service.

The options `User=` and `Group=` are not useful for mount units. systemd passes two parameters to
mount(8) the values of `What=` and `Where=`. When invoked in this way, _mount(8)_ does not read any
options from `/etc/fstab`, and must be run as UID 0.

Mount units must be named after the mount point directories they control. For instance, the mount
point `/home/lennart` must be configured in a unit file `home-lennart.mount`. For details about the
escaping logic used to convert a file system path to a unit name, see _systemd.unit(5)_. Note that
mount units cannot be templated, nor is possible to add multiple names to a mount unit by creating
additional symlinks to it.

Mount units may either be configured via unit files, or via `/etc/fstab` (see `man fstab` for
details). Mounts listed in /etc/fstab will be converted into native units dynamically at boot and
when the configuration of the system manager is reloaded. In general, configuring mount points
through `/etc/fstab` is the preferred approach. See _systemd-fstab-generator(8)_ for details about
the
conversion. <sup> [2](https://www.freedesktop.org/software/systemd/man/systemd.mount.html) </sup>

</details>


<details>
<summary>References and source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [man page for systemd.mount](https://www.freedesktop.org/software/systemd/man/systemd.mount.html)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a mount is in failed state, you should always try to gather more information about it.

1. Identify which mount units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_mount_units.mount_unit_state`). In this chart, identify which mount
   units are in state with value 5

2. Gather more information about the failing mount. We advise you to run the following command
   in a second terminal.

   ```
   root@netdata~ # journalctl -u <mount_name>.mount -f 
   ```
   This command will monitor the journalctl log messages for your mount unit.
3. In your main terminal, try mount the mount manually.

   ```
   root@netdata~ # mount -v <mount_name> 
   ```

   This command will try to mount your mount unit in verbose mode.
4. Check the output messages from both terminals for abnormalities.

</details>


<details>
<summary>Verify the fstab configuration</summary>

1. Open a terminal and run the following command 

   ```
   root@netdata~ # sudo findmnt --verify --verbose
   ```

This command will check mount table content (default: `/etc/fstab`) in verbose mode 
</details>
