# systemd_slice_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd slice units state. The `systemd_slice_units_state` alert
indicates that one or more of the systemd slice units are in the failed state.
A systemd slice unit "failed" when the service process returned error code on exit, or crashed, an 
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
<summary>See more on systemd-slice</summary>

The following text originates from the systemd.slice man page.<sup>[2](https://www.freedesktop.org/software/systemd/man/systemd.slice.html) </sup>

A unit configuration file whose name ends in ".slice" encodes information about a slice unit. A
slice unit is a concept for hierarchically managing resources of a group of processes. This
management is performed by creating a node in the Linux Control Group (cgroup) tree. Units that
manage processes (primarily scope and service units) may be assigned to a specific slice. For each
slice, certain resource limits may be set that apply to all processes of all units contained in that
slice. Slices are organized hierarchically in a tree. The name of the slice encodes the location in
the tree. The name consists of a dash-separated series of names, which describes the path to the
slice from the root slice. The root slice is named -.slice. For example, foo-bar.slice is a slice
that is located within foo.slice, which in turn is located in the root slice -.slice.

Note that slice units cannot be templated, nor is possible to add multiple names to a slice unit by
creating additional symlinks to its unit file.

By default, service and scope units are placed in `system.slice`, virtual machines and containers
registered with `systemd-machined` are found in `machine.slice`, and user sessions handled by
`systemd-logind` in `user.slice`.

The slice specific configuration options are configured in the `[Slice]` section. Currently, only
generic resource control settings as described in systemd.resource-control(5) are allowed.

</details>


<details>

<summary>References and source</summary>

1. [systemd on Wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [Man page for systemd.slice](https://www.freedesktop.org/software/systemd/man/systemd.slice.html)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a slice is in a failed state, you should always try to gather more information about it.

1. Identify which slice units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_slice_units.slice_unit_state`). In this chart, identify which slice
   units are in state with value 5.

2. Gather more information about the failing slice unit

   ```
   root@netdata~ # systemctl status <slice_name>.slice
   ```

3. Check the log messages from the command of step 2.

</details>
