# Standard System Monitoring Plugins

Internally the following plugins have been implemented:

 - `/proc/net/dev` (all network interfaces for all their values)
 - `/proc/diskstats` (all disks for all their values)
 - `/proc/net/snmp` (total IPv4, TCP and UDP usage)
 - `/proc/net/snmp6` (total IPv6 usage)
 - `/proc/net/netstat` (more IPv4 usage)
 - `/proc/net/stat/nf_conntrack` (connection tracking performance)
 - `/proc/net/stat/synproxy` (synproxy performance)
 - `/proc/net/ip_vs/stats` (IPVS connection statistics)
 - `/proc/stat` (CPU utilization)
 - `/proc/meminfo` (memory information)
 - `/proc/vmstat` (system performance)
 - `/proc/net/rpc/nfsd` (NFS server statistics for both v3 and v4 NFS servers)
 - `/proc/interrupts` (total and per core hardware interrupts)
 - `/proc/softirqs` (total and per core software interrupts)
 - `/proc/loadavg` (system load and total processes running)
 - `/proc/sys/kernel/random/entropy_avail` (random numbers pool availability - used in cryptography)
 - `ksm` Kernel Same-Page Merging performance (several files under `/sys/kernel/mm/ksm`).
 - `netdata` (internal netdata resources utilization)

In case this page is left behind in updates, the source code for runs these internal plugins is [here](https://github.com/firehol/netdata/blob/master/src/plugin_proc.c).


## QoS

Netdata monitors `tc` QoS classes for all interfaces.

If you also use [FireQOS](http://firehol.org/tutorial/fireqos-new-user/)) it will collect interface and class names.

There is a [shell helper](https://github.com/firehol/netdata/blob/master/plugins.d/tc-qos-helper.sh) for this (all parsing is done by the plugin in `C` code - this shell script is just a configuration for the command to run to get `tc` output).

The source of the tc plugin is [here](https://github.com/firehol/netdata/blob/master/src/plugin_tc.c). It is somewhat complex, because a state machine was needed to keep track of all the `tc` classes, including the pseudo classes tc dynamically creates.


## Netfilter Accounting

There is also a plugin that collects NFACCT statistics. This plugin is currently disabled by default, because it requires root access. I have to move the code to an external plugin to setuid just the plugin not the whole netdata server.

You can build netdata with it to test it though. Just run `./configure` with the option `--enable-plugin-nfacct` (and any other options you may need). Remember, you have to tell netdata you want it to run as `root` for this plugin to work.

## Idle Jitter

Idle jitter is calculated by netdata. It works like this:

A thread is spawned that requests to sleep for a few microseconds. When the system wakes it up, it measures how many microseconds have passed. The difference between the requested and the actual duration of the sleep, is the idle jitter.

This number is useful in real-time environments, where CPU jitter can affect the quality of the service (like VoIP media gateways).

The source code is [here](https://github.com/firehol/netdata/blob/master/src/plugin_idlejitter.c).
