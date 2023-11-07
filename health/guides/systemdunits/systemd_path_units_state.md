# systemd_path_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd path units state. This alert indicates that one or more of
the systemd path units are in the failed state.
A systemd path unit "failed" when the service process returned error code on exit, or crashed, an 
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
<summary>See more on systemd.path units</summary>

> A unit configuration file whose name ends in ".path" encodes information about a path monitored by system. With path units, you can monitor files and directories for certain events. If a specified event occurs, a service unit is executed, and it usually carries the same name as the path unit.
>
> In the [Path] section, `PathChanged=` specifies the absolute path to the file to be monitored, while
`Unit=` indicates which service unit to execute if the file changes. <sup>[2](https://www.redhat.com/sysadmin/introduction-path-units) </sup>

Path units are very useful to monitor files for changes with systemd.
</details>


<details>
<summary>References and source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [A Brief introduction to path units by Jörg Kastning](https://www.redhat.com/sysadmin/introduction-path-units)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a path is in failed state, you should always try to gather more information about it.

1. Identify which path units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_path_units.path_unit_state`). In this chart, identify which path units are in
   state with value 5

2. Gather more information about the failing path unit and the service that manages it (in most of
   the cases they will have the same name). We advise you to run the following commands in two
   different terminals.

   ```
   root@netdata~ # journalctl -u <service_name>.service -f
   root@netdata~ # journalctl -u <path_name>.socket -f
   ```
   These commands will monitor the journalctl log messages for your path/service unit.
3. In a new terminal, try to restart the service.

   ```
   root@netdata~ # systemctl restart <service_name>.service 
   ```

4. Check the log messages from the commands of step 2.

</details>

<details>
<summary>Run the service of the path unit in debug mode</summary>

1. Identify which path units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_path_units.path_unit_state`). In this chart, identify which path units are in
   state with value 5

2. Stop the service that manages this path (in most of the cases the service will have the same name
   with the path)

   ```
   root@netdata~ # systemctl stop <service_name>.service 
   ```

3. Try to start it with the `SYSTEMD_LOG_LEVEL=debug` env variable. Let's assume in our case we want
   to debug the `systemd-networkd` service

   ```
   root@netdata~ # SYSTEMD_LOG_LEVEL=debug /lib/systemd/systemd-networkd

   ```

4. Check the log messages.

</details>
