# systemd_device_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the `systemd.device` units. The `systemd_device_units_state` alert
indicates that one or more of the `systemd.device` units have failed.
A systemd device unit "failed" when the service process returned error code on exit, or crashed, an 
operation timed out, or after too many restarts. The cause of a failed states is stored in a log.

<details>
<summary>Read More About systemd</summary>

Here is some useful information about systemd from
Wikipedia <sup>[1](https://en.wikipedia.org/wiki/Systemd) </sup>

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
<summary>More On systemd-`.device` Units</summary>

A unit configuration file whose name ends in `.device` encodes information about a device unit as
exposed in the `sysfs/udev(7)` device tree. `udev` provides a dynamic device directory containing
only the files for actually present devices. It creates or removes device node files in the `/dev`
directory, or it renames network interfaces.

This unit type has no specific options. Device units are named after the `/sys` and `/dev` paths
they control. For example, the device `/dev/sda5` is exposed in systemd as `dev-sda5.device`.
</details>

<details>
<summary>References and Source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [man page for systemd.device](https://www.freedesktop.org/software/systemd/man/systemd.device.html)

</details>

### Troubleshooting Section:

<details>
<summary>General Approach</summary>

If a device has failed, then you should always try to gather more information about the cause of the failure.

1. Identify which device units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart
   (`systemdunits_device_units.device_unit_state`). In this chart, identify which device
   units are in state with value 5.

2. Check the status of the device

   ```
   root@netdata~ # systemctl status  <device_name>.device 
   ```
   
</details>
