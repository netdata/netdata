# systemd_service_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd service units. The `systemd_service_units_state` alert
indicates that one or more of the systemd service units are in the `failed` state. One of the
following reasons can cause this alert:

- The process of the service returns an error code on exit.
- The process of the service crashed.
- An operation timed out occurred.
- The service failed after too many restarts.

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
<summary>See more on systemd-services</summary>

A unit configuration file whose name ends in `.service` encodes information about a process
controlled and supervised by systemd. To `view`, `start`, `stop`, `restart`, `enable`, or `disable`
system services, use the `systemctl` command line interface. It is common that services are ordered
to start after some specified service that depends on (try the
command `systemctl list-dependencies --before|after <service_name>.service`)

See more in the man pages, `man systemd.service`

</details>

<details>
<summary>References and source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a service is in failed state, you should always try to gather more information about it.

1. Identify which service units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_service_units.service_unit_state`). In this chart, identify which service
   units are in state with value 5

2. Gather more information about the failing service. We advise you to run the following command in
   a second terminal.

   ```
   root@netdata~ # journalctl -u <service_name>.service -f 
   ```
   This command will monitor the journalctl log messages for your service.

3. In a new terminal, try to restart the service.

   ```
   root@netdata~ # systemctl restart <service_name>.service 
   ```
   This command will restart your service.

4. Check the log messages from the command of step 2.

</details>

<details>
<summary>Run the service in debug mode</summary>

1. Identify which service units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   (`systemdunits_service_units.service_unit_state`). In this chart, identify which service
   units are in state with value 5


2. Stop the service

   ```
   root@netdata~ # systemctl stop <service_name>.service 
   ```

3. Try to start it with the `SYSTEMD_LOG_LEVEL=debug` env variable. Let's assume in our case we want
   to debug the `systemd-networkd` service.

   ```
   root@netdata~ # SYSTEMD_LOG_LEVEL=debug /lib/systemd/systemd-networkd

   ```
   This command will start your service in debug mode.
4. Check the log messages.

</details>
