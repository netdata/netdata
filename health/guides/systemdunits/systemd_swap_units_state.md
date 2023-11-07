# systemd_swap_units_state

**Linux | Systemd units**

_Systemd is a suite of basic building blocks for a Linux system. It provides a system and service
manager that runs as PID 1 and starts the rest of the system._

The Netdata Agent monitors the systemd swap units state. The `systemd_swap_units_state` alert
indicates that one or more of the systemd swap units are in the failed state.
A systemd swap unit "failed" when the service process returned error code on exit, or crashed, an 
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
<summary>See more on systemd-swap</summary>
The following text originates from the systemd.swap man page.<sup>[2](https://www.freedesktop.org/software/systemd/man/systemd.swap.html) </sup>

A unit configuration file whose name ends in `.swap` encodes information about a swap device or
file for memory paging controlled and supervised by systemd. Swap units must be named after the
devices or files they control. For instance, the swap device `/dev/sda5` must be configured in a
unit file`dev-sda5.swap`. Note that swap units cannot be templated, nor is possible to add multiple 
names to a swap unit by creating additional symlinks to it.
Swap units may either be configured via unit files, or via `/etc/fstab` (see `man fstab(5)` for
details). Swaps listed in `/etc/fstab` will be converted into native units dynamically at boot and
when the configuration of the system manager is reloaded. See `man systemd-fstab-generator` for
details about the conversion.
If a swap device or file is configured in both `/etc/fstab` and a unit file, the configuration in
the latter takes precedence.
When reading `/etc/fstab`, a few special options are understood by systemd which influence how
dependencies are created for swap units. With `noauto`, the swap unit will not be added as a
dependency for `swap.target`. This means that it will not be activated automatically during boot,
unless it is pulled in by some other unit. The auto option has the opposite meaning and is the
default. With `nofail`, the swap unit will be only wanted, not required by `swap.target`. This means
that the boot will continue even if this swap device is not activated 
successfully. 

</details>


<details>
<summary>References and source</summary>

1. [Systemd on Wikipedia](https://en.wikipedia.org/wiki/Systemd)
2. [Man page for systemd.swap](https://www.freedesktop.org/software/systemd/man/systemd.swap.html)

</details>

### Troubleshooting section:

<details>
<summary>General approach</summary>

Check the log messages for failing reasons:

   ```
   root@netdata # journalctl -xe | grep -A 5 -B 5 swap
   ```

</details>


<details>
<summary>Check your fstab for errors</summary>

Open the fstab config file and verify the syntax of the fstab entries with `TYPE=swap` are
    correct.

   ```
   root@netdata # vim /etc/fstab
   ```

    Consult the [man pages of fstab](https://www.man7.org/linux/man-pages/man5/fstab.5.html) for 
    misconfigurations.

</details>
