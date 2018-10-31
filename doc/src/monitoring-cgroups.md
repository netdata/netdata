# cgroups

You can monitor containers and virtual machines using **cgroups**.

cgroups (or control groups), are a Linux kernel feature that provides accounting and resource usage limiting for processes. When cgroups are bundled with namespaces (i.e. isolation), they form what we usually call **containers**.

cgroups are hierarchical, meaning that cgroups can contain child cgroups, which can contain more cgroups, etc. All accounting is reported (and resource usage limits are applied) also in a hierarchical way.

To visualize cgroup metrics netdata provides configuration for cherry picking the cgroups of interest. By default (without any configuration) netdata should pick **systemd services**, all kinds of **containers** (lxc, docker, etc) and **virtual machines** spawn by managers that register them with cgroups (qemu, libvirt, etc).

## configuring netdata for cgroups

For each cgroup available in the system, netdata provides this configuration:

```
[plugin:cgroups]
    enable cgroup XXX = yes | no
```

But it also provides a few patterns to provide a sane default (`yes` or `no`).

Below we see, how this works.

### how netdata finds the available cgroups
Linux exposes resource usage reporting and provides dynamic configuration for cgroups, using virtual files (usually) under `/sys/fs/cgroup`. netdata reads `/proc/self/mountinfo` to detect the exact mount point of cgroups. netdata also allows manual configuration of this mount point, using these settings:

```
[plugin:cgroups]
	check for new cgroups every = 10
	path to /sys/fs/cgroup/cpuacct = /sys/fs/cgroup/cpuacct
	path to /sys/fs/cgroup/blkio = /sys/fs/cgroup/blkio
	path to /sys/fs/cgroup/memory = /sys/fs/cgroup/memory
	path to /sys/fs/cgroup/devices = /sys/fs/cgroup/devices
``` 

netdata rescans these directories for added or removed cgroups every `check for new cgroups every` seconds.


### hierarchical search for cgroups

Since cgroups are hierarchical, for each of the directories shown above, netdata walks through the subdirectories recursively searching for cgroups (each subdirectory is another cgroup).

For each of the directories found, netdata provides a configuration variable:

```
[plugin:cgroups]
	search for cgroups under PATH = yes | no
```

To provide a sane default for this setting, netdata uses the following pattern list (patterns starting with `!` give a negative match and their order is important: the first matching a path will be used):

```
[plugin:cgroups]
	search for cgroups in subpaths matching =  !*/init.scope  !*-qemu  !/init.scope  !/system  !/systemd  !/user  !/user.slice  * 
```

So, we disable checking for **child cgroups** in systemd internal cgroups ([systemd services are monitored by netdata](https://github.com/netdata/netdata/wiki/monitoring-systemd-services)), user cgroups (normally used for desktop and remote user sessions), qemu virtual machines (child cgroups of virtual machines) and `init.scope`. All others are enabled.


### enabled cgroups

To check if the cgroup is enabled, netdata uses this setting:

```
[plugin:cgroups]
	enable cgroup NAME = yes | no
```

To provide a sane default, netdata uses the following pattern list (it checks the pattern against the path of the cgroup):

```
[plugin:cgroups]
	enable by default cgroups matching =  !*/init.scope  *.scope  !*/vcpu*  !*/emulator  !*.mount  !*.partition  !*.service  !*.slice  !*.swap  !*.user  !/  !/docker  !/libvirt  !/lxc  !/lxc/*/ns  !/lxc/*/ns/*  !/machine  !/qemu  !/system  !/systemd  !/user  * 
```

The above provides the default `yes` or `no` setting for the cgroup. However, there is an additional step. In many cases the cgroups found in the `/sys/fs/cgroup` hierarchy are just random numbers and in many cases these numbers are ephemeral: they change across reboots or sessions.

So, we need to somehow map the paths of the cgroups to names, to provide consistent netdata configuration (i.e. there is no point to say `enable cgroup 1234 = yes | no`, if `1234` is a random number that changes over time - we need a name for the cgroup first, so that `enable cgroup NAME = yes | no` will be consistent).

For this mapping netdata provides 2 configuration options:

```
[plugin:cgroups]
	run script to rename cgroups matching =  *.scope  *docker*  *lxc*  *qemu*  !/  !*.mount  !*.partition  !*.service  !*.slice  !*.swap  !*.user  *
	script to get cgroup names = /usr/libexec/netdata/plugins.d/cgroup-name.sh
```

The whole point for the additional pattern list, is to limit the number of times the script will be called. Without this pattern list, the script might be called thousands of times, depending on the number of cgroups available in the system.

The above pattern list is matched against the path of the cgroup. For matched cgroups, netdata calls the script [cgroup-name.sh](https://github.com/netdata/netdata/blob/master/plugins.d/cgroup-name.sh) to get its name. This script queries `docker`, or applies heuristics to find give a name for the cgroup.

## Ephemeral Containers

netdata monitors containers automatically when it is installed at the host, or when it is installed in a container that has access to the `/proc` and `/sys` filesystems of the host.

netdata prior to v1.6 had 2 issues when such containers were monitored:

1. network interface alarms where triggering when containers were stopped

2. charts were never cleaned up, so after some time dozens of containers were showing up on the dashboard, and they were occupying memory.

Now, network interfaces and cgroups (containers) are self-cleaned.

So, when a network interface or container stops, netdata might log a few errors in error.log complaining about files it cannot find, but immediately:

1. it will detect this is a removed container or network interface
2. it will freeze/pause all alarms for them
3. it will mark their charts as obsolete
4. obsolete charts are not be offered on new dashboard sessions (so hit F5 and the charts are gone)
5. existing dashboard sessions will continue to see them, but of course they will not refresh
6. obsolete charts will be removed from memory, 1 hour after the last user viewed them (configurable with `[global].cleanup obsolete charts after seconds = 3600` (at netdata.conf).
7. when obsolete charts are removed from memory they are also deleted from disk (configurable with `[global].delete obsolete charts files = yes`)
