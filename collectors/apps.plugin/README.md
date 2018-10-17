# apps.plugin

`apps.plugin` breaks down system resource usage to **processes**, **users** and **user groups**.

To achieve this task, it iterates through the whole process tree, collecting resource usage information
for every process found running.

Since netdata needs to present this information in charts and track them through time,
instead of presenting a `top` like list, `apps.plugin` uses a pre-defined list of **process groups**
to which it assigns all running processes. This list is [customizable](apps_groups.conf) and netdata
ships with a good default for most cases (to edit it on your system run `/etc/netdata/edit-config apps_groups.conf`).

So, `apps.plugin` builds a process tree (much like `ps fax` does in Linux), and groups
processes together (evaluating both child and parent processes) so that the result is always a list with
a predefined set of members (of course, only process groups found running are reported).

> If you find that `apps.plugin` categorizes standard applications as `other`, we would be
> glad to accept pull requests improving the [defaults](apps_groups.conf) shipped with netdata.

Unlike traditional process monitoring tools (like `top`), `apps.plugin` is able to account the resource
utilization of exit processes. Their utilization is accounted at their currently running parents.
So, `apps.plugin` is perfectly able to measure the resources used by shell scripts and other processes
that fork/spawn other short lived processes hundreds of times per second.

For example, ssh to a server running netdata and execute this:

```sh
while true; do ls -l /var/run >/dev/null; done
```

All the console tools will report that a a CPU core is 100% used, but they will fail to identify which
process is using all that CPU (because there is no single process using it - thousands of `ls` per second
are using it). Netdata however, will be able to identify that `ssh` is using it
(`ssh` is the parent process group defined in its [default config](apps_groups.conf)):

![](https://cloud.githubusercontent.com/assets/2662304/21076220/c9687848-bf2e-11e6-8d81-348592c5aca2.png)

This feature makes `apps.plugin` unique in narrowing down the list of offending processes that may be
responsible for slow downs, or abusing system resources.

## Charts

`apps.plugin` provides charts for 3 sections:

1. Per application charts as **Applications** at netdata dashboards
2. Per user charts as **Users** at netdata dashboards
3. Per user group charts as **User Groups** at netdata dashboards

Each of these sections provides the same number of charts:

- CPU Utilization
    - Total CPU usage
    - User / System CPU usage
- Disk I/O
    - Physical Reads / Writes
    - Logical Reads / Writes
    - Open Unique Files (if a file is found open multiple times, it is counted just once)
- Memory
    - Real Memory Used (non shared)
    - Virtual Memory Allocated
    - Minor Page Faults (i.e. memory activity)
- Processes
    - Threads Running
    - Processes Running
    - Pipes Open
- Swap Memory
    - Swap Memory Used
    - Major Page Faults (i.e. swap activity)
- Network
    - Sockets Open

The above are reported:

- For **Applications** per [target configured](apps_groups.conf).
- For **Users** per username or UID (when the username is not available).
- For **User Groups** per groupname or GID (when groupname is not available).

## Performance

`apps.plugin` is a complex piece of software and has a lot of work to do
We are proud that `apps.plugin` is a lot faster compared to any other similar tool,
while collecting a lot more information for the processes, however the fact is that
this plugin requires more CPU resources than the netdata daemon itself.

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

Uncomment the line `update every` and set it to a higher number. If you just set it to ` 2 `,
its CPU resources will be cut in half, and data collection will be once every 2 seconds.

## Configuration

The configuration file is `/etc/netdata/apps_groups.conf` (the default is [here](apps_groups.conf)).
To edit it on your system run `/etc/netdata/edit-config apps_groups.conf`.

The configuration file works accepts multiple lines, each having this format:

```txt
group: process1 process2 ...
```

Each group can be given multiple times, to add more processes to it.

For the **Applications** section, only groups configured in this file are reported.
All other processes will be reported as `other`.

For each process given, its whole process tree will be grouped, not just the process matched.
The plugin will include both parents and children.

The process names are the ones returned by:

  - `ps -e` or `cat /proc/PID/stat`
  - in case of substring mode (see below): `/proc/PID/cmdline`

To add process names with spaces, enclose them in quotes (single or double)
example: ` 'Plex Media Serv' ` or ` "my other process" `.

You can add an asterisk ` * ` at the beginning and/or the end of a process:

  - `*name` *suffix* mode: will search for processes ending with `name` (at `/proc/PID/stat`)
  - `name*` *prefix* mode: will search for processes beginning with `name` (at `/proc/PID/stat`)
  - `*name*` *substring* mode: will search for `name` in the whole command line (at `/proc/PID/cmdline`)

If you enter even just one *name* (substring), `apps.plugin` will process
`/proc/PID/cmdline` for all processes (of course only once per process: when they are first seen).

To add processes with single quotes, enclose them in double quotes: ` "process with this ' single quote" `

To add processes with double quotes, enclose them in single quotes: ` 'process with this " double quote' `

If a group or process name starts with a ` - `, the dimension will be hidden from the chart (cpu chart only).

If a process starts with a ` + `, debugging will be enabled for it (debugging produces a lot of output - do not enable it in production systems).

You can add any number of groups. Only the ones found running will affect the charts generated.
However, producing charts with hundreds of dimensions may slow down your web browser.

The order of the entries in this list is important: the first that matches a process is used, so put important
ones at the top. Processes not matched by any row, will inherit it from their parents or children.

The order also controls the order of the dimensions on the generated charts (although applications started
after apps.plugin is started, will be appended to the existing list of dimensions the netdata daemon maintains).

## Permissions

`apps.plugin` requires additional privileges to collect all the information it needs.
The problem is described in issue #157.

When netdata is installed, `apps.plugin` is given the capabilities `cap_dac_read_search,cap_sys_ptrace+ep`.
If this fails (i.e. `setcap` fails), `apps.plugin` is setuid to `root`.

#### linux capabilities in containers

There are a few cases, like `docker` and `virtuozzo` containers, where `setcap` succeeds, but the capabilities
are silently ignored (in `lxc` containers `setcap` fails).

In these cases ()`setcap` succeeds but capabilities do not work), you will have to setuid
to root `apps.plugin` by running these commands:

```sh
chown root:netdata /usr/libexec/netdata/plugins.d/apps.plugin
chmod 4750 /usr/libexec/netdata/plugins.d/apps.plugin
```

You will have to run these, every time you update netdata.

## Security

`apps.plugin` performs a hard-coded function of building the process tree in memory,
iterating forever, collecting metrics for each running process and sending them to netdata.
This is a one-way communication, from `apps.plugin` to netdata.

So, since `apps.plugin` cannot be instructed by netdata for the actions it performs,
we think it is pretty safe to allow it have these increased privileges.

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

Netdata is able give you a lot more badges for your app.
Examples below for process group `sql`:

- CPU usage: ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.cpu&dimensions=sql&value_color=green=0%7Corange%3C50%7Cred)
- Disk Physical Reads ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.preads&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Disk Physical Writes ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.pwrites&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Disk Logical Reads ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.lreads&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Disk Logical Writes ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.lwrites&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Open Files ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.files&dimensions=sql&value_color=green%3E30%7Cred)
- Real Memory ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.mem&dimensions=sql&value_color=green%3C100%7Corange%3C200%7Cred)
- Virtual Memory ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.vmem&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Swap Memory ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.swap&dimensions=sql&value_color=green=0%7Cred)
- Minor Page Faults ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.minor_faults&dimensions=sql&value_color=green%3C100%7Corange%3C1000%7Cred)
- Processes ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.processes&dimensions=sql&value_color=green%3E0%7Cred)
- Threads ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.threads&dimensions=sql&value_color=green%3E=28%7Cred)
- Major Faults (swap activity) ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.major_faults&dimensions=sql&value_color=green=0%7Cred)
- Open Pipes ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.pipes&dimensions=sql&value_color=green=0%7Cred)
- Open Sockets ![image](http://registry.my-netdata.io/api/v1/badge.svg?chart=apps.sockets&dimensions=sql&value_color=green%3E=3%7Cred)


For more information about badges check [Generating Badges](https://github.com/netdata/netdata/wiki/Generating-Badges)