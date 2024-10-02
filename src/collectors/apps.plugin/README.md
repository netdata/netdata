<!--
title: "Application monitoring (apps.plugin)"
sidebar_label: "Application monitoring "
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/apps.plugin/README.md"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/System metrics"
-->

# Applications monitoring (apps.plugin)

`apps.plugin` monitors the resources utilization of all processes running.

## Process Aggregation and Grouping

`apps.plugin` aggregates processes in three distinct ways to provide a more insightful
breakdown of resource utilization:

 - **Tree** or **Category**: Grouped by their position in the process tree.
   This is customizable and allows aggregation by process managers and individual
   processes of interest. Allows also renaming the processes for presentation purposes.
 
 - **User**: Grouped by the effective user (UID) under which the processes run.
 
 - **Group**: Grouped by the effective group (GID) under which the processes run.

 ## Short-Lived Process Handling

`apps.plugin` accounts for resource utilization of both running and exited processes,
capturing the impact of processes that spawn short-lived subprocesses, such as shell
scripts that fork hundreds or thousands of times per second. So, although processes
may spawn short lived sub-processes, `apps.plugin` will aggregate their resources
utilization providing a holistic view of how resources are shared among the processes. 

## Charts sections

To provide more valuable insights, apps.plugin aggregates individual processes in several ways.
Each type of aggregation is presented as a different section on the dashboard.

### Custom Process Groups (Apps)

In this section, apps.plugin summarizes the resources consumed by all processes, grouped based
on the groups provided in `/etc/netdata/apps_groups.conf`. You can edit this file using our [`edit-config`](docs/netdata-agent/configuration/README.md) script.

For this section, `apps.plugin` builds a process tree (much like `ps fax` does in Linux), and groups
processes together (evaluating both child and parent processes) so that the result is always a list with
a predefined set of members (of course, only process groups found running are reported).

> If you find that `apps.plugin` categorizes standard applications as `other`, we would be
> glad to accept pull requests improving the defaults shipped with Netdata in `apps_groups.conf`.

### By User (Users)

In this section, apps.plugin summarizes the resources consumed by all processes, grouped by the
effective user under which each process runs.

### By User Group (Groups)

In this section, apps.plugin summarizes the resources consumed by all processes, grouped by the
effective user group under which each process runs.

## Charts

`apps.plugin` provides charts for 3 sections:

1.  Per application charts as **Applications** at Netdata dashboards
2.  Per user charts as **Users** at Netdata dashboards
3.  Per user group charts as **User Groups** at Netdata dashboards

Each of these sections provides the same number of charts:

-   CPU utilization (`apps.cpu`)
    -   Total CPU usage
    -   User/system CPU usage (`apps.cpu_user`/`apps.cpu_system`)
-   Disk I/O
    -   Physical reads/writes (`apps.preads`/`apps.pwrites`)
    -   Logical reads/writes (`apps.lreads`/`apps.lwrites`)
    -   Open unique files (if a file is found open multiple times, it is counted just once, `apps.files`)
-   Memory
    -   Real Memory Used (non-shared, `apps.mem`)
    -   Virtual Memory Allocated (`apps.vmem`)
    -   Minor page faults (i.e. memory activity, `apps.minor_faults`)
-   Processes
    -   Threads running (`apps.threads`)
    -   Processes running (`apps.processes`)
    -   Carried over uptime (since the last Netdata Agent restart, `apps.uptime`)
    -   Minimum uptime (`apps.uptime_min`)
    -   Average uptime (`apps.uptime_average`)
    -   Maximum uptime (`apps.uptime_max`)
    -   Pipes open (`apps.pipes`)
-   Swap memory
    -   Swap memory used (`apps.swap`)
    -   Major page faults (i.e. swap activity, `apps.major_faults`)
-   Network
    -   Sockets open (`apps.sockets`)
    
In addition, if the [eBPF collector](/src/collectors/ebpf.plugin/README.md) is running, your dashboard will also show an
additional [list of charts](/src/collectors/ebpf.plugin/README.md#integration-with-appsplugin) using low-level Linux
metrics.

The above are reported:

-   For **Applications** per target configured.
-   For **Users** per username or UID (when the username is not available).
-   For **User Groups** per group name or GID (when group name is not available).

## Performance

`apps.plugin` is a complex piece of software and has a lot of work to do
We are proud that `apps.plugin` is a lot faster compared to any other similar tool,
while collecting a lot more information for the processes, however the fact is that
this plugin may require more CPU resources than the `netdata` daemon itself.

Under Linux, for each process running, `apps.plugin` reads several `/proc` files
per process. Doing this work per-second, especially on hosts with several thousands
of processes, may increase the CPU resources consumed by the plugin.

In such cases, you many need to lower its data collection frequency.

To do this, edit `/etc/netdata/netdata.conf` and find this section:

```
[plugin:apps]
	# update every = 1
	# command options =
```

Uncomment the line `update every` and set it to a higher number. If you just set it to `2`,
its CPU resources will be cut in half, and data collection will be once every 2 seconds.

## Configuration

The configuration file is `/etc/netdata/apps_groups.conf`. You can edit this file using our [`edit-config`](docs/netdata-agent/configuration/README.md) script.

### Configuring process managers

`apps.plugin` needs to know the common process managers, meaning the names of the processes
which spawn other processes. Process managers are used so that `apps.plugin` will automatically
consider all their sub-processes important to monitor.

Process managers are configured in `apps_groups.conf` with the prefix `managers:`, like this:

```
managers: process1 process2 process3
```

Multiple lines may exist, all starting with `managers:`.

The process names given here should be exactly as the operating system sets them. In Linux these
process names are limited to 15 characters. Usually the command `ps -e` or `cat /proc/{PID}/stat`
states the names needed here.

### Configuring process groups and renaming processes

The configuration file works accepts multiple lines, each having this format:

```txt
group: process1 process2 ...
```

Each group can be given multiple times, to add more processes to it.

For each process given, all of its sub-processes will be grouped, not just the matched process.

The process names are the ones returned by:

-   **comm**: `ps -e` or `cat /proc/{PID}/stat`
-   **cmdline**: in case of substring mode (see below): `/proc/{PID}/cmdline`

On Linux **comm** is limited to just a few characters. `apps.plugin` attempts to find the entire
**comm** name by looking for it at the **cmdline**. When this is successful, the entire process name
is available, otherwise the shortened one is used.

To add process names with spaces, enclose them in quotes (single or double)
example: `'Plex Media Serv'` or `"my other process"`.

You can add asterisks (`*`) to provide a pattern:

-   `*name` _suffix_ mode: will match a **comm** ending with `name`.
-   `name*` _prefix_ mode: will match a **comm** beginning with `name`.
-   `*name*` _substring_ mode: will search for `name` in **cmdline**.

Asterisks may appear in the middle of `name` (like `na*me`), without affecting what is being
matched (**comm** or **cmdline**). 

To add processes with single quotes, enclose them in double quotes: `"process with this ' single quote"`

To add processes with double quotes, enclose them in single quotes: `'process with this " double quote'`

The order of the entries in this list is important: the first one that matches a process is used, so follow a top-down hierarchy.
Processes not matched by any row, will inherit it from their parents.

There are a few command line options you can pass to `apps.plugin`. The list of available
options can be acquired with the `--help` flag. The options can be set in the `netdata.conf` using the [`edit-config` script](/docs/netdata-agent/configuration/README.md).
For example, to disable user and user group charts you would set:

```
[plugin:apps]
  command options = without-users without-groups
```

### Integration with eBPF

If you don't see charts under the **eBPF syscall** or **eBPF net** sections, you should edit your
[`ebpf.d.conf`](/src/collectors/ebpf.plugin/README.md#configure-the-ebpf-collector) file to ensure the eBPF program is enabled.

Also see our [guide on troubleshooting apps with eBPF
metrics](/docs/developer-and-contributor-corner/monitor-debug-applications-ebpf.md) for ideas on how to interpret these charts in a
few scenarios.

## Permissions

`apps.plugin` requires additional privileges to collect all the information it needs.
The problem is described in issue #157.

When Netdata is installed, `apps.plugin` is given the capabilities `cap_dac_read_search,cap_sys_ptrace+ep`.
If this fails (i.e. `setcap` fails), `apps.plugin` is setuid to `root`.

### linux capabilities in containers

There are a few cases, like `docker` and `virtuozzo` containers, where `setcap` succeeds, but the capabilities
are silently ignored (in `lxc` containers `setcap` fails).

In this case, you will have to setuid to root `apps.plugin` by running these commands:

```sh
chown root:netdata /usr/libexec/netdata/plugins.d/apps.plugin
chmod 4750 /usr/libexec/netdata/plugins.d/apps.plugin
```

You will have to run these, every time you update Netdata.

## Security

`apps.plugin` performs a hard-coded function of building the process tree in memory,
iterating forever, collecting metrics for each running process and sending them to Netdata.
This is a one-way communication, from `apps.plugin` to Netdata.

So, since `apps.plugin` cannot be instructed by Netdata for the actions it performs,
we think it is pretty safe to allow it to have these increased privileges.

Keep in mind that `apps.plugin` will still run without escalated permissions,
but it will not be able to collect all the information.

## Application Badges

You can create badges that you can embed anywhere you like, with URLs like this:

```
https://your.netdata.ip:19999/api/v1/badge.svg?chart=apps.processes&dimensions=myapp&value_color=green%3E0%7Cred
```

The color expression unescaped is this: `value_color=green>0|red`.

Here is an example for the process group `sql` at `https://registry.my-netdata.io`:

![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.processes&dimensions=sql&value_color=green%3E0%7Cred)

Netdata is able to give you a lot more badges for your app.
Examples below for process group `sql`:

-   CPU usage: ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.cpu&dimensions=sql&value_color=green=0%7Corange%3C50%7Cred)
-   Disk Physical Reads ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.preads&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Disk Physical Writes ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.pwrites&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Disk Logical Reads ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.lreads&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Disk Logical Writes ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.lwrites&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Open Files ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.fds_files&dimensions=sql&value_color=green%3E30%7Cred)
-   Real Memory ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.mem&dimensions=sql&value_color=green%3C100%7Corange%3C200%7Cred)
-   Virtual Memory ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.vmem&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Swap Memory ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.swap&dimensions=sql&value_color=green=0%7Cred)
-   Minor Page Faults ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.minor_faults&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
-   Processes ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.processes&dimensions=sql&value_color=green%3E0%7Cred)
-   Threads ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.threads&dimensions=sql&value_color=green%3E=28%7Cred)
-   Major Faults (swap activity) ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.major_faults&dimensions=sql&value_color=green=0%7Cred)
-   Open Pipes ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.fds_pipes&dimensions=sql&value_color=green=0%7Cred)
-   Open Sockets ![image](https://registry.my-netdata.io/api/v1/badge.svg?chart=apps.fds_sockets&dimensions=sql&value_color=green%3E=3%7Cred)

For more information about badges check [Generating Badges](/src/web/api/v2/api_v3_badge/README.md)

## Comparison with console tools

SSH to a server running Netdata and execute this:

```sh
while true; do ls -l /var/run >/dev/null; done
```

In most systems `/var/run` is a `tmpfs` device, so there is nothing that can stop this command
from consuming entirely one of the CPU cores of the machine.

As we will see below, **none** of the console performance monitoring tools can report that this
command is using 100% CPU. They do report of course that the CPU is busy, but **they fail to
identify the process that consumes so much CPU**.

Here is what common Linux console monitoring tools report:

### top

`top` reports that `bash` is using just 14%.

If you check the total system CPU utilization, it says there is no idle CPU at all, but `top`
fails to provide a breakdown of the CPU consumption in the system. The sum of the CPU utilization
of all processes reported by `top`, is 15.6%.

```
top - 18:46:28 up 3 days, 20:14,  2 users,  load average: 0.22, 0.05, 0.02
Tasks:  76 total,   2 running,  74 sleeping,   0 stopped,   0 zombie
%Cpu(s): 32.8 us, 65.6 sy,  0.0 ni,  0.0 id,  0.0 wa,  1.3 hi,  0.3 si,  0.0 st
KiB Mem :  1016576 total,   244112 free,    52012 used,   720452 buff/cache
KiB Swap:        0 total,        0 free,        0 used.   753712 avail Mem

  PID USER      PR  NI    VIRT    RES    SHR S %CPU %MEM     TIME+ COMMAND
12789 root      20   0   14980   4180   3020 S 14.0  0.4   0:02.82 bash
    9 root      20   0       0      0      0 S  1.0  0.0   0:22.36 rcuos/0
  642 netdata   20   0  132024  20112   2660 S  0.3  2.0  14:26.29 netdata
12522 netdata   20   0    9508   2476   1828 S  0.3  0.2   0:02.26 apps.plugin
    1 root      20   0   67196  10216   7500 S  0.0  1.0   0:04.83 systemd
    2 root      20   0       0      0      0 S  0.0  0.0   0:00.00 kthreadd
```

### htop

Exactly like `top`, `htop` is providing an incomplete breakdown of the system CPU utilization.

```
  CPU[||||||||||||||||||||||||100.0%]   Tasks: 27, 11 thr; 2 running
  Mem[||||||||||||||||||||85.4M/993M]   Load average: 1.16 0.88 0.90
  Swp[                         0K/0K]   Uptime: 3 days, 21:37:03

  PID USER      PRI  NI  VIRT   RES   SHR S CPU% MEM%   TIME+  Command
12789 root       20   0 15104  4484  3208 S 14.0  0.4 10:57.15 -bash
 7024 netdata    20   0  9544  2480  1744 S  0.7  0.2  0:00.88 /usr/libexec/netd
 7009 netdata    20   0  138M 21016  2712 S  0.7  2.1  0:00.89 /usr/sbin/netdata
 7012 netdata    20   0  138M 21016  2712 S  0.0  2.1  0:00.31 /usr/sbin/netdata
  563 root	     20   0  308M  202M  202M S  0.0 20.4  1:00.81 /usr/lib/systemd/
 7019 netdata    20   0  138M 21016  2712 S  0.0  2.1  0:00.14 /usr/sbin/netdata
```

### atop

`atop` also fails to break down CPU usage.

```
ATOP - localhost            2016/12/10  20:11:27    -----------      10s elapsed
PRC | sys    1.13s | user   0.43s | #proc     75 | #zombie    0 | #exit   5383 |
CPU | sys      67% | user     31% | irq       2% | idle      0% | wait      0% |
CPL | avg1    1.34 | avg5    1.05 | avg15   0.96 | csw    51346 | intr   10508 |
MEM | tot   992.8M | free  211.5M | cache 470.0M | buff   87.2M | slab  164.7M |
SWP | tot     0.0M | free    0.0M |              | vmcom 207.6M | vmlim 496.4M |
DSK |          vda | busy      0% | read       0 | write      4 | avio 1.50 ms |
NET | transport    | tcpi      16 | tcpo      15 | udpi       0 | udpo       0 |
NET | network      | ipi       16 | ipo       15 | ipfrw      0 | deliv     16 |
NET | eth0    ---- | pcki      16 | pcko      15 | si    1 Kbps | so    4 Kbps |

  PID SYSCPU USRCPU   VGROW  RGROW  RDDSK   WRDSK ST EXC  S  CPU CMD       1/600
12789  0.98s  0.40s      0K     0K     0K    336K --   -  S  14% bash
    9  0.08s  0.00s      0K     0K     0K      0K --   -  S   1% rcuos/0
 7024  0.03s  0.00s      0K     0K     0K      0K --   -  S   0% apps.plugin
 7009  0.01s  0.01s	     0K     0K     0K      4K --   -  S   0% netdata
```

### glances

And the same is true for `glances`. The system runs at 100%, but `glances` reports only 17%
per process utilization.

Note also, that being a `python` program, `glances` uses 1.6% CPU while it runs.

```
localhost                                               Uptime: 3 days, 21:42:00

CPU  [100.0%]   CPU     100.0%   MEM     23.7%   SWAP      0.0%   LOAD    1-core
MEM  [ 23.7%]   user:    30.9%   total:   993M   total:       0   1 min:    1.18
SWAP [  0.0%]   system:  67.8%   used:    236M   used:        0   5 min:    1.08
                idle:     0.0%   free:    757M   free:        0   15 min:   1.00

NETWORK     Rx/s   Tx/s   TASKS  75 (90 thr), 1 run, 74 slp, 0 oth
eth0        168b    2Kb
eth1          0b     0b     CPU%  MEM%   PID USER        NI S Command
lo            0b     0b     13.5   0.4 12789 root         0 S -bash
                             1.6   2.2  7025 root         0 R /usr/bin/python /u
DISK I/O     R/s    W/s      1.0   0.0     9 root         0 S rcuos/0
vda1           0     4K      0.3   0.2  7024 netdata      0 S /usr/libexec/netda
                             0.3   0.0     7 root         0 S rcu_sched
FILE SYS    Used  Total      0.3   2.1  7009 netdata      0 S /usr/sbin/netdata
/ (vda1)   1.56G  29.5G      0.0   0.0    17 root         0 S oom_reaper
```

### why does this happen?

All the console tools report usage based on the processes found running *at the moment they
examine the process tree*. So, they see just one `ls` command, which is actually very quick
with minor CPU utilization. But the shell, is spawning hundreds of them, one after another
(much like shell scripts do).

### What does Netdata report?

The total CPU utilization of the system:

![image](https://cloud.githubusercontent.com/assets/2662304/21076212/9198e5a6-bf2e-11e6-9bc0-6bdea25befb2.png)
<br/>***Figure 1**: The system overview section at Netdata, just a few seconds after the command was run*

And at the applications `apps.plugin` breaks down CPU usage per application:

![image](https://cloud.githubusercontent.com/assets/2662304/21076220/c9687848-bf2e-11e6-8d81-348592c5aca2.png)
<br/>***Figure 2**: The Applications section at Netdata, just a few seconds after the command was run*

So, the `ssh` session is using 95% CPU time.

Why `ssh`?

`apps.plugin` groups all processes based on its configuration file.
The default configuration has nothing for `bash`, but it has for `sshd`, so Netdata accumulates
all ssh sessions to a dimension on the charts, called `ssh`. This includes all the processes in
the process tree of `sshd`, **including the exited children**.

> Distributions based on `systemd`, provide another way to get cpu utilization per user session
> or service running: control groups, or cgroups, commonly used as part of containers
> `apps.plugin` does not use these mechanisms. The process grouping made by `apps.plugin` works
> on any Linux, `systemd` based or not.

#### a more technical description of how apps.plugin works

Apps.plugin reads `/proc/<pid>/stat` for all processes, once per second and extracts `utime` and
`stime` (user and system cpu utilization), much like all the console tools do.

But it also extracts `cutime` and `cstime` that account the user and system time of the exit
children of each process. By keeping a map in memory of the whole process tree, it is capable of
assigning the right time to every process, taking into account all its exited children.

It is tricky, since a process may be running for 1 hour and once it exits, its parent should not
receive the whole 1 hour of cpu time in just 1 second - you have to subtract the cpu time that has
been reported for it prior to this iteration.

It is even trickier, because walking through the entire process tree takes some time itself. So,
if you sum the CPU utilization of all processes, you might have more CPU time than the reported
total cpu time of the system. Netdata solves this, by adapting the per process cpu utilization to
the total of the system. [Apps.plugin adds charts that document this normalization](https://london.my-netdata.io/default.html#menu_netdata_submenu_apps_plugin).
