# apps.plugin

This plugin provides charts for 3 sections of the default dashboard:

1. Per application charts
2. Per user charts
3. Per user group charts

## Per application charts

This plugin walks through the entire `/proc` filesystem and aggregates statistics for applications of interest, defined in `/etc/netdata/apps_groups.conf` (the default is [here](apps_groups.conf)) (to edit it on your system run `/etc/netdata/edit-config apps_groups.conf`).

The plugin internally builds a process tree (much like `ps fax` does), and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported).

Using this information it provides the following charts (per application group defined in `/etc/netdata/apps_groups.conf` - to edit it on your system run `/etc/netdata/edit-config apps_groups.conf`):

1. Total CPU usage
2. Total User CPU usage
3. Total System CPU usage
4. Total Disk Physical Reads
5. Total Disk Physical Writes
6. Total Disk Logical Reads
7. Total Disk Logical Writes
8. Total Open Files (unique files - if a file is found open multiple times, it is counted just once)
9. Total Dedicated Memory (non shared)
10. Total Minor Page Faults
11. Total Number of Processes
12. Total Number of Threads
13. Total Number of Pipes
14. Total Swap Activity (Major Page Faults)
15. Total Open Sockets

## Per User Charts

All the above charts, are also grouped by username, using the effective uid of each process.

## Per Group Charts

All the above charts, are also grouped by group name, using the effective gid of each process.

## CPU Usage

`apps.plugin` is a complex software piece and has a lot of work to do (actually this plugin requires more CPU resources that the netdata daemon). For each process running, `apps.plugin` reads several `/proc` files to get CPU usage, memory allocated, I/O usage, open file descriptors, etc. Doing this work per-second, especially on hosts with several thousands of processes, may increase the CPU resources consumed by the plugin.

In such cases, you many need to lower its data collection frequency. To do this, edit `/etc/netdata/netdata.conf` and find this section:

```
[plugin:apps]
	# update every = 1
	# command options = 
```

Uncomment the line `update every` and set it to a higher number. If you just set it to ` 2 `, its CPU resources will be cut in half, and data collection will be once every 2 seconds.


## Configuration

The configuration file is `/etc/netdata/apps_groups.conf` (the default is [here](apps_groups.conf)).
To edit it on your system run `/etc/netdata/edit-config apps_groups.conf`.

The configuration file works accepts multiple lines, each having this format:

```txt
group: process1 process2 ...
```

Process names should be given as they appear when running `ps -e`. The program will actually match the process names in the `/proc/PID/status` file. So, to be sure the name is right for a process running with PID ` X `, do this:

```sh
cat /proc/X/status
```

The first line on the output is `Name: xxxxx`. This is the process name `apps.plugin` sees.

The order of the lines in the file is important only if you include the same process name to multiple groups.

## Apps plugin is missing information

`apps.plugin` requires additional privileges to collect all the information it needs. The problem is described in issue #157.

When netdata is installed, `apps.plugin` is given the capabilities `cap_dac_read_search,cap_sys_ptrace+ep`. If that is not possible (i.e. `setcap` fails), `apps.plugin` is setuid to `root`.

## linux capabilities in containers

There are a few cases, like `docker` and `virtuozzo` containers, where `setcap` succeeds, but the capabilities are silently ignored (in `lxc` containers `setcap` fails).

In these cases that `setcap` succeeds by capabilities do not work, you will have to setuid to root `apps.plugin` by running these commands:

```sh
chown root:netdata /usr/libexec/netdata/plugins.d/apps.plugin
chmod 4750 /usr/libexec/netdata/plugins.d/apps.plugin
```

You will have to run these, every time you update netdata.


### Is is safe to give `apps.plugin` these privileges?

`apps.plugin` performs a hard-coded function of building the process tree in memory, iterating forever, collecting metrics for each running process and sending them to netdata. This is a one-way communication, from `apps.plugin` to netdata.

So, since `apps.plugin` cannot be instructed by netdata for the actions it performs, we think it is pretty safe to allow it have these increased privileges.

Keep in mind that `apps.plugin` will still run without these permissions, but it will not be able to collect all the data for every process.
