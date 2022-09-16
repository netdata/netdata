<!--
title: "cgroups.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/cgroups.plugin/README.md
-->

# cgroups.plugin

You can monitor containers and virtual machines using **cgroups**.

cgroups (or control groups), are a Linux kernel feature that provides accounting and resource usage limiting for
processes. When cgroups are bundled with namespaces (i.e. isolation), they form what we usually call **containers**.

cgroups are hierarchical, meaning that cgroups can contain child cgroups, which can contain more cgroups, etc. All
accounting is reported (and resource usage limits are applied) also in a hierarchical way.

To visualize cgroup metrics Netdata provides configuration for cherry picking the cgroups of interest. By default (
without any configuration) Netdata should pick **systemd services**, all kinds of **containers** (lxc, docker, etc)
and **virtual machines** spawn by managers that register them with cgroups (qemu, libvirt, etc).

## Configuring Netdata for cgroups

In general, no additional settings are required. Netdata discovers all available cgroups on the host system and
collects their metrics.

### how Netdata finds the available cgroups

Linux exposes resource usage reporting and provides dynamic configuration for cgroups, using virtual files (usually)
under `/sys/fs/cgroup`. Netdata reads `/proc/self/mountinfo` to detect the exact mount point of cgroups. Netdata also
allows manual configuration of this mount point, using these settings:

```text
[plugin:cgroups]
	check for new cgroups every = 10
	path to /sys/fs/cgroup/cpuacct = /sys/fs/cgroup/cpuacct
	path to /sys/fs/cgroup/blkio = /sys/fs/cgroup/blkio
	path to /sys/fs/cgroup/memory = /sys/fs/cgroup/memory
	path to /sys/fs/cgroup/devices = /sys/fs/cgroup/devices
```

Netdata rescans these directories for added or removed cgroups every `check for new cgroups every` seconds.

### hierarchical search for cgroups

Since cgroups are hierarchical, for each of the directories shown above, Netdata walks through the subdirectories
recursively searching for cgroups (each subdirectory is another cgroup).

To provide a sane default for this setting, Netdata uses the following pattern list (patterns starting with `!` give a
negative match and their order is important: the first matching a path will be used):

```text
[plugin:cgroups]
	search for cgroups in subpaths matching =  !*/init.scope  !*-qemu  !/init.scope  !/system  !/systemd  !/user  !/user.slice  *
```

So, we disable checking for **child cgroups** in systemd internal
cgroups ([systemd services are monitored by Netdata](#monitoring-systemd-services)), user cgroups (normally used for
desktop and remote user sessions), qemu virtual machines (child cgroups of virtual machines) and `init.scope`. All
others are enabled.

### unified cgroups (cgroups v2) support

Netdata automatically detects cgroups version. If detection fails Netdata assumes v1.
To switch to v2 manually add:

```text
[plugin:cgroups]
	use unified cgroups = yes
	path to unified cgroups = /sys/fs/cgroup
```

Unified cgroups use same name pattern matching as v1 cgroups. `cgroup_enable_systemd_services_detailed_memory` is
currently unsupported when using unified cgroups.

### enabled cgroups

To provide a sane default, Netdata uses the
following [pattern list](https://learn.netdata.cloud/docs/agent/libnetdata/simple_pattern):

- checks the pattern against the path of the cgroup

  ```text
  [plugin:cgroups]
  	enable by default cgroups matching =  !*/init.scope  *.scope  !*/vcpu*  !*/emulator  !*.mount  !*.partition  !*.service  !*.slice  !*.swap  !*.user  !/  !/docker  !/libvirt  !/lxc  !/lxc/*/ns  !/lxc/*/ns/*  !/machine  !/qemu  !/system  !/systemd  !/user  *
  ```

- checks the pattern against the name of the cgroup (as you see it on the dashboard)

  ```text
  [plugin:cgroups]
  	enable by default cgroups names matching = *
  ```

Renaming is configured with the following options:

```text
[plugin:cgroups]
	run script to rename cgroups matching =  *.scope  *docker*  *lxc*  *qemu*  !/  !*.mount  !*.partition  !*.service  !*.slice  !*.swap  !*.user  *
	script to get cgroup names = /usr/libexec/netdata/plugins.d/cgroup-name.sh
```

The whole point for the additional pattern list, is to limit the number of times the script will be called. Without this
pattern list, the script might be called thousands of times, depending on the number of cgroups available in the system.

The above pattern list is matched against the path of the cgroup. For matched cgroups, Netdata calls the
script [cgroup-name.sh](https://raw.githubusercontent.com/netdata/netdata/master/collectors/cgroups.plugin/cgroup-name.sh)
to get its name. This script queries `docker`, `kubectl`, `podman`, or applies heuristics to find give a name for the
cgroup.

#### Note on Podman container names

Podman's security model is a lot more restrictive than Docker's, so Netdata will not be able to detect container names
out of the box unless they were started by the same user as Netdata itself.

If Podman is used in "rootful" mode, it's also possible to use `podman system service` to grant Netdata access to
container names. To do this, ensure `podman system service` is running and Netdata has access
to `/run/podman/podman.sock` (the default permissions as specified by upstream are `0600`, with owner `root`, so you
will have to adjust the configuration).

[docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) can also be used to give Netdata restricted
access to the socket. Note that `PODMAN_HOST` in Netdata's environment should be set to the proxy's URL in this case.

### charts with zero metrics

By default, Netdata will enable monitoring metrics only when they are not zero. If they are constantly zero they are
ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be
automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though). Set `yes` for a
chart instead of `auto` to enable it permanently. For example:

```text
[plugin:cgroups]
	enable memory (used mem including cache) = yes
```

You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero
metrics for all internal Netdata plugins.

### alarms

CPU and memory limits are watched and used to rise alarms. Memory usage for every cgroup is checked against `ram`
and `ram+swap` limits. CPU usage for every cgroup is checked against `cpuset.cpus` and `cpu.cfs_period_us` + `cpu.cfs_quota_us` pair assigned for the cgroup. Configuration for the alarms is available in `health.d/cgroups.conf`
file.

## Monitoring systemd services

Netdata monitors **systemd services**. Example:

![image](https://cloud.githubusercontent.com/assets/2662304/21964372/20cd7b84-db53-11e6-98a2-b9c986b082c0.png)

Support per distribution:

|      system      | charts shown |        `/sys/fs/cgroup` tree         | comments                  |
|:----------------:|:------------:|:------------------------------------:|:--------------------------|
|    Arch Linux    |     YES      |                                      |                           |
|      Gentoo      |      NO      |                                      | can be enabled, see below |
| Ubuntu 16.04 LTS |     YES      |                                      |                           |
|   Ubuntu 16.10   |     YES      | [here](http://pastebin.com/PiWbQEXy) |                           |
|    Fedora 25     |     YES      | [here](http://pastebin.com/ax0373wF) |                           |
|     Debian 8     |      NO      |                                      | can be enabled, see below |
|       AMI        |      NO      | [here](http://pastebin.com/FrxmptjL) | not a systemd system      |
| CentOS 7.3.1611  |      NO      | [here](http://pastebin.com/SpzgezAg) | can be enabled, see below |

### Monitored systemd service metrics

- CPU utilization
- Used memory
- RSS memory
- Mapped memory
- Cache memory
- Writeback memory
- Memory minor page faults
- Memory major page faults
- Memory charging activity
- Memory uncharging activity
- Memory limit failures
- Swap memory used
- Disk read bandwidth
- Disk write bandwidth
- Disk read operations
- Disk write operations
- Throttle disk read bandwidth
- Throttle disk write bandwidth
- Throttle disk read operations
- Throttle disk write operations
- Queued disk read operations
- Queued disk write operations
- Merged disk read operations
- Merged disk write operations

### how to enable cgroup accounting on systemd systems that is by default disabled

You can verify there is no accounting enabled, by running `systemd-cgtop`. The program will show only resources for
cgroup `/`, but all services will show nothing.

To enable cgroup accounting, execute this:

```sh
sed -e 's|^#Default\(.*\)Accounting=.*$|Default\1Accounting=yes|g' /etc/systemd/system.conf >/tmp/system.conf
```

To see the changes it made, run this:

```sh
# diff /etc/systemd/system.conf /tmp/system.conf
40,44c40,44
< #DefaultCPUAccounting=no
< #DefaultIOAccounting=no
< #DefaultBlockIOAccounting=no
< #DefaultMemoryAccounting=no
< #DefaultTasksAccounting=yes
---
> DefaultCPUAccounting=yes
> DefaultIOAccounting=yes
> DefaultBlockIOAccounting=yes
> DefaultMemoryAccounting=yes
> DefaultTasksAccounting=yes
```

If you are happy with the changes, run:

```sh
# copy the file to the right location
sudo cp /tmp/system.conf /etc/systemd/system.conf

# restart systemd to take it into account
sudo systemctl daemon-reexec
```

(`systemctl daemon-reload` does not reload the configuration of the server - so you have to
execute `systemctl daemon-reexec`).

Now, when you run `systemd-cgtop`, services will start reporting usage (if it does not, restart any service to wake it up). Refresh your Netdata dashboard, and you will have the charts too.

In case memory accounting is missing, you will need to enable it at your kernel, by appending the following kernel boot
options and rebooting:

```sh
cgroup_enable=memory swapaccount=1
```

You can add the above, directly at the `linux` line in your `/boot/grub/grub.cfg` or appending them to
the `GRUB_CMDLINE_LINUX` in `/etc/default/grub` (in which case you will have to run `update-grub` before rebooting). On
DigitalOcean debian images you may have to set it at `/etc/default/grub.d/50-cloudimg-settings.cfg`.

Which systemd services are monitored by Netdata is determined by the following pattern list:

```text
[plugin:cgroups]
	cgroups to match as systemd services =  !/system.slice/*/*.service  /system.slice/*.service
```

- - -

## Monitoring ephemeral containers

Netdata monitors containers automatically when it is installed at the host, or when it is installed in a container that
has access to the `/proc` and `/sys` filesystems of the host.

Netdata prior to v1.6 had 2 issues when such containers were monitored:

1. network interface alarms where triggering when containers were stopped

2. charts were never cleaned up, so after some time dozens of containers were showing up on the dashboard, and they were
   occupying memory.

### the current Netdata

network interfaces and cgroups (containers) are now self-cleaned.

So, when a network interface or container stops, Netdata might log a few errors in error.log complaining about files it
cannot find, but immediately:

1. it will detect this is a removed container or network interface
2. it will freeze/pause all alarms for them
3. it will mark their charts as obsolete
4. obsolete charts are not be offered on new dashboard sessions (so hit F5 and the charts are gone)
5. existing dashboard sessions will continue to see them, but of course they will not refresh
6. obsolete charts will be removed from memory, 1 hour after the last user viewed them (configurable
   with `[global].cleanup obsolete charts after seconds = 3600` (at `netdata.conf`).
7. when obsolete charts are removed from memory they are also deleted from disk (configurable
   with `[global].delete obsolete charts files = yes`)

### Monitored container metrics

- CPU usage
- CPU usage within the limits
- CPU usage per core
- Memory usage
- Writeback memory
- Memory activity
- Memory page faults
- Used memory
- Used RAM within the limits
- Memory utilization
- Memory limit failures
- I/O bandwidth (all disks)
- Serviced I/O operations (all disks)
- Throttle I/O bandwidth (all disks)
- Throttle serviced I/O operations (all disks)
- Queued I/O operations (all disks)
- Merged I/O operations (all disks)
- CPU pressure
- Memory pressure
- Memory full pressure
- I/O pressure
- I/O full pressure

Network interfaces are monitored by means of
the [proc plugin](/collectors/proc.plugin/README.md#monitored-network-interface-metrics).
