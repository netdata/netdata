# Standard System Monitoring Plugins

Internally the following plugins have been implemented:

 - `/proc/net/dev` (all network interfaces for all their values)
 - `/proc/diskstats` (all disks for all their values)
 - `/proc/net/snmp` (total IPv4, TCP and UDP usage)
 - `/proc/net/snmp6` (total IPv6 usage)
 - `/proc/net/netstat` (more IPv4 usage)
 - `/proc/net/stat/nf_conntrack` (connection tracking performance)
 - `/proc/net/stat/synproxy` ([synproxy](#linux-anti-ddos) performance)
 - `/proc/net/ip_vs/stats` (IPVS connection statistics)
 - `/proc/stat` (CPU utilization)
 - `/proc/meminfo` (memory information)
 - `/proc/vmstat` (system performance)
 - `/proc/net/rpc/nfsd` (NFS server statistics for both v3 and v4 NFS servers)
 - `/sys/fs/cgroup` (Control Groups - Linux Containers)
 - `/proc/self/mountinfo` (mount points)
 - `/proc/interrupts` (total and per core hardware interrupts)
 - `/proc/softirqs` (total and per core software interrupts)
 - `/proc/loadavg` (system load and total processes running)
 - `/proc/sys/kernel/random/entropy_avail` (random numbers pool availability - used in cryptography)
 - `ksm` Kernel Same-Page Merging performance (several files under `/sys/kernel/mm/ksm`).
 - `netdata` (internal netdata resources utilization)

In case this page is left behind in updates, the source code for runs these internal plugins is [here](https://github.com/netdata/netdata/blob/master/src/plugin_proc.c).


## QoS

Netdata [monitors `tc` QoS](QoS) classes for all interfaces.

If you also use [FireQOS](http://firehol.org/tutorial/fireqos-new-user/) it will collect interface and class names.

There is a [shell helper](https://github.com/netdata/netdata/blob/master/plugins.d/tc-qos-helper.sh) for this (all parsing is done by the plugin in `C` code - this shell script is just a configuration for the command to run to get `tc` output).

The source of the tc plugin is [here](https://github.com/netdata/netdata/blob/master/src/plugin_tc.c). It is somewhat complex, because a state machine was needed to keep track of all the `tc` classes, including the pseudo classes tc dynamically creates.


## Netfilter Accounting

There is also a plugin that collects NFACCT statistics. This plugin is currently disabled by default, because it requires root access. I have to move the code to an external plugin to setuid just the plugin not the whole netdata server.

You can build netdata with it to test it though. Just run `./configure` with the option `--enable-plugin-nfacct` (and any other options you may need). Remember, you have to tell netdata you want it to run as `root` for this plugin to work.

## Idle Jitter

Idle jitter is calculated by netdata. It works like this:

A thread is spawned that requests to sleep for a few microseconds. When the system wakes it up, it measures how many microseconds have passed. The difference between the requested and the actual duration of the sleep, is the idle jitter.

This number is useful in real-time environments, where CPU jitter can affect the quality of the service (like VoIP media gateways).

The source code is [here](https://github.com/netdata/netdata/blob/master/src/plugin_idlejitter.c).

## Linux Anti-DDoS

![image6](https://cloud.githubusercontent.com/assets/2662304/14253733/53550b16-fa95-11e5-8d9d-4ed171df4735.gif)

---
SYNPROXY is a TCP SYN packets proxy. It can be used to protect any TCP server (like a web server) from SYN floods and similar DDos attacks.

SYNPROXY is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections.

The net effect of this, is that the real servers will not notice any change during the attack. The valid TCP connections will pass through and served, while the attack will be stopped at the firewall.

To use SYNPROXY on your firewall, please follow our setup guides:

 - **[Working with SYNPROXY](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY)**
 - **[Working with SYNPROXY and traps](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY-and-traps)**

### Real-time monitoring of Linux Anti-DDoS

netdata is able to monitor in real-time (per second updates) the operation of the Linux Anti-DDoS protection.

It visualizes 4 charts:

1. TCP SYN Packets received on ports operated by SYNPROXY
2. TCP Cookies (valid, invalid, retransmits)
3. Connections Reopened
4. Entries used

Example image:

![ddos](https://cloud.githubusercontent.com/assets/2662304/14398891/6016e3fc-fdf0-11e5-942b-55de6a52cb66.gif)

See Linux Anti-DDoS in action at: **[netdata demo site (with SYNPROXY enabled)](http://london.my-netdata.io/#netfilter_synproxy)** 

