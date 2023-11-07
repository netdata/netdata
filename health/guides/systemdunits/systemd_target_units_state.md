# systemd_target_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd target units state. Receiving this alert indicates that one
or more of the systemd target units are in the failed state. A systemd target unit "failed" when 
the service process returned error code on exit, or crashed, an operation timed out, or after 
too many restarts. The cause of a failed states is stored in a log.

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
<summary>See more on systemd.target units</summary>

> Target units file ends with the `.target` file extension and their only purpose is to group together
other systemd units through a chain of dependencies. For example, the graphical.target unit, which
is used to start a graphical session, starts system services such as the GNOME Display Manager (
gdm.service) or Accounts Service (accounts-daemon .service) and also activates the multi-user.target
unit. Similarly, the multi-user.target unit starts other essential system services such as
NetworkManager (NetworkManager.service) or D-Bus (dbus.service) and activates another target unit
named
basic.target. <sup> [2](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/configuring_basic_system_settings/working-with-systemd-targets_configuring-basic-system-settings) </sup>
>
> Among other things, target units are a more flexible replacement for SysV runlevels in the classic
SysV init system. For compatibility reasons special target units such as runlevel3.target exist
which are used by the SysV runlevel compatibility code in systemd. 

</details>


<details>
<summary>References and source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [systemd.target explained on Redhat's documentantion](https://access.redhat.com/documentation/en-us/red_hat_enterprise_linux/8/html/configuring_basic_system_settings/working-with-systemd-targets_configuring-basic-system-settings)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a target is in a failed state, you should always try to gather more information about it.

1. Identify which target units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_target_units.target_unit_state`). In this chart, identify which target
   units are in state with value 5

2. Gather more information about the failing target unit

   ```
   root@netdata~ # systemctl status <target_name>.target
   ```

3. Check the log messages from the command of step 2.

</details>
