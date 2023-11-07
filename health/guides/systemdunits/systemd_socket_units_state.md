# systemd_socket_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd socket units state. Receiving this alerts indicates that one
or more of the systemd socket units are in the failed state. In most of the cases this is correlated
with the service,  which manages the socket.
A systemd socket unit "failed" when the service process returned error code on exit, or crashed, an 
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
<summary>See more on systemd.socket units</summary>

> A unit configuration file whose name ends in `.socket` encodes information about an IPC or network socket or a file system FIFO controlled and supervised by systemd, for socket-based activation. For each socket unit, a matching service unit must exist, describing the service to start on incoming traffic on the socket. The name of the .service unit is by default the same as the name of the .socket unit.
>
> Note that the daemon software configured for socket activation with socket units needs to be able to accept sockets from systemd, either via systemd's native socket passing interface (see sd_listen_fds(3) for details about the precise protocol used and the order in which the file descriptors are passed) or via traditional inetd(8)-style socket passing (i.e. sockets passed in via standard input and output, using StandardInput=socket in the service file).
>
> All network sockets allocated through .socket units are allocated in the host's network namespace
(see network_namespaces(7)). This does not mean however that the service activated by a configured socket unit has to be part of the host's network namespace as well. It is supported and even good practice to run services in their own network namespace (for example through PrivateNetwork=, see systemd.exec(5)), receiving only the sockets configured through socket-activation from the host's namespace. In such a set-up communication within the host's network namespace is only permitted through the activation sockets passed in while all sockets allocated from the service code itself will be associated with the service's own namespace, and thus possibly subject to a a much more restrictive configuration. <sup>[2](https://manpages.debian.org/testing/systemd/systemd.socket.5.en.html) </sup>

</details>


<details>
<summary>References and source</summary>

1. [systemd on wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [systemd.socket on debian.org](https://manpages.debian.org/testing/systemd/systemd.socket.5.en.html)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

When a socket is in failed state, you should always try to gather more information about it.

1. Identify which socket units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   `systemdunits_service_units.socket_unit_state` chart. Check which sockets are in state with value 5.
    

2. Gather more information about the failing socket and the service that manages it (in most of the 
   cases they will have the same name). We advise you to run the following commands in two different
   terminals.

   ```
   root@netdata~ # journalctl -u <service_name>.service -f
   root@netdata~ # journalctl -u <socket_name>.socket -f
   ```
   
   These commands will monitor the journalctl log messages for your socket/service unit.
3. In a new terminal, try to restart the service.

   ```
   root@netdata~ # systemctl restart <service_name>.service 
   ```

4. Check the log messages from the command of step 2.

</details>

<details>
<summary>Run the service of the socket in debug mode</summary>

1. Identify which socket units fail. Open the Netdata dashboard, find the current active alarms under
   the [active alarms](https://learn.netdata.cloud/docs/monitor/view-active-alarms) tab and look
   into its chart.
   `systemdunits_service_units.socket_unit_state` chart. Check which sockets are in state with value 5.

2. Stop the service that manages this socket (in most of the cases the service will have the same 
   name with the socket)

   ```
   root@netdata~ # systemctl stop <service_name>.service 
   ```

3. Try to start it with the `SYSTEMD_LOG_LEVEL=debug` env variable. Let's assume in our case we want
   to debug the `systemd-networkd` service

   ```
   root@netdata~ # SYSTEMD_LOG_LEVEL=debug /lib/systemd/systemd-networkd

   ```

   This command will start your service in debug mode.
4. Check the log messages.

</details>
