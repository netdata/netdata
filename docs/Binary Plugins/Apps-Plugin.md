# Apps.plugin

This plugin provides charts for 3 sections of the default dashboard:

1. Per application charts
2. Per user charts
3. Per user group charts

## Per application charts

This plugin walks through the entire `/proc` filesystem and aggregates statistics for applications of interest, defined in `/etc/netdata/apps_groups.conf` (the default is [here](https://github.com/firehol/netdata/blob/master/conf.d/apps_groups.conf)).

The plugin internally builds a process tree (much like `ps fax` does), and groups processes together (evaluating both child and parent processes) so that the result is always a chart with a predefined set of dimensions (of course, only application groups found running are reported).

Using this information it provides the following charts (per application group defined in `/etc/netdata/apps_groups.conf`):

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

## Limitations
The values gathered here are not 100% accurate. They only include values for the processes **running**.

If an application is spawning children continuously, which are terminated in just a few milliseconds (like shell scripts do), the values reported will be inaccurate. Linux does report the values for the exited childs of a process. However, these values are reported to the parent process only when the child exits. If these values, of the exited child processes, were also aggregated in the charts below, the charts would have been full of spikes, presenting unrealistic utilization for each process group. So, we decided to ignore these values and present only the utilization of the currently running processes.

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

The configuration file is `/etc/netdata/apps_groups.conf` (the default is [here](https://github.com/firehol/netdata/blob/master/conf.d/apps_groups.conf)).

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