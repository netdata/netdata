# systemd_scope_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd scope units state. The `systemd_scope_units_state` alert
indicates that one or more of the systemd-scope units are in the failed state.
A systemd scope unit "failed" when the service process returned error code on exit, or crashed, an 
operation timed out, or after too many restarts. The cause of a failed states is stored in a log.

<details>
<summary>Read more about systemd</summary>

Here is some useful information about systemd from
wikipedia <sup>[1](https://en.wikipedia.org/wiki/Systemd) </sup>

Systemd includes features like on-demand starting of daemons, snapshot support, process tracking,
and Inhibitor Locks. Systemd is not just the name of the `init` daemon, but also refers to the
entire software bundle around it, which, in addition to the systemd `init` daemon, includes the
daemons
`journald`, `logind` and `networkd`, and many other low-level components. In January 2013,
Poettering described systemd not as one program, but rather a large software suite that includes 69
individual binaries. As an integrated software suite, systemd replaces the startup sequences and
runlevels controlled by the traditional `init` daemon, along with the shell scripts executed under
its control. Systemd also integrates many other services that are common on Linux systems by
handling user logins, the system console, device hotplugging, scheduled execution (replacing cron),
logging, hostnames and locales.

Like the `init daemon`, Systemd is a daemon that manages other daemons, which, including `systemd`
itself, are background processes. `systemd` is the first daemon to start during booting and the last
daemon to terminate during shutdown. The `systemd` daemon serves as the root of the user space's
process tree. The first process (`PID 1`) has a special role on Unix systems, as it replaces the
parent of a process when the original parent terminates. Therefore, the first process is
particularly well suited for the purpose of monitoring daemons.

Systemd executes elements of its startup sequence in parallel, which is theoretically faster than
the traditional startup sequence approach. For inter-process communication (IPC), systemd makes Unix
domain sockets and D-Bus available to the running daemons. The state of `systemd` itself can also be
preserved in a snapshot for future recall.

systemd's core components include the following:

- `systemd` is a system and service manager for Linux operating systems.

- `systemctl` is a command to introspect and control the state of the systemd system and service
  manager. Not to be confused with sysctl.

- `systemd-analyze` may be used to determine system boot-up performance statistics and retrieve
  other state and tracing information from the system and service manager.

</details>


<details>
<summary>See more on systemd-scope</summary>
The following text originates from the systemd.scope man page.<sup>[2](https://www.freedesktop.org/software/systemd/man/systemd.scope.html) </sup>

Scope units are not configured via unit configuration files, but are only created programmatically
using the bus interfaces of systemd. They are named similar to filenames. A unit whose name ends
in `.scope` refers to a scope unit. Scopes units manage a set of system processes. Unlike service
units, scope units manage externally created processes, and do not fork off processes on its own.

The main purpose of scope units is grouping worker processes of a system service for organization
and for managing resources.

Unlike service units, scope units have no "main" process: all processes in the scope are equivalent.
The lifecycle of the scope unit is thus not bound to the lifetime of one specific process, but to
the existence of at least one process in the scope. This also means that the exit statuses of these
processes are not relevant for the scope unit failure state. Scope units may still enter a failure
state, for example due to resource exhaustion or stop timeouts being reached, but not due to
programs inside of them terminating uncleanly. Since processes managed as scope units generally
remain children of the original process that forked them off, it is also the job of that process to
collect their exit statuses and act on them as
needed.

</details>

<details>
<summary>References and source</summary>

1. [systemd on Wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [Man page for systemd.scope](https://www.freedesktop.org/software/systemd/man/systemd.scope.html)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a scope is in a failed state, you should always try to gather more information about it.

1. Identify which scope units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_scope_units.scope_unit_state`). In this chart, identify which slice units are in
   state with value 5

2. Gather more information about the failing scope unit

   ```
   root@netdata~ # systemctl status <scope_name>.scope
   ```

3. Check the log messages from the command of step 2.

</details>
