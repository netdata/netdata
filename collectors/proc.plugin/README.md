<!--
title: "proc.plugin"
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/proc.plugin/README.md
-->

# proc.plugin

-   `/proc/net/dev` (all network interfaces for all their values)
-   `/proc/diskstats` (all disks for all their values)
-   `/proc/mdstat` (status of RAID arrays)
-   `/proc/net/snmp` (total IPv4, TCP and UDP usage)
-   `/proc/net/snmp6` (total IPv6 usage)
-   `/proc/net/netstat` (more IPv4 usage)
-   `/proc/net/wireless` (wireless extension)
-   `/proc/net/stat/nf_conntrack` (connection tracking performance)
-   `/proc/net/stat/synproxy` (synproxy performance)
-   `/proc/net/ip_vs/stats` (IPVS connection statistics)
-   `/proc/stat` (CPU utilization and attributes)
-   `/proc/meminfo` (memory information)
-   `/proc/vmstat` (system performance)
-   `/proc/net/rpc/nfsd` (NFS server statistics for both v3 and v4 NFS servers)
-   `/sys/fs/cgroup` (Control Groups - Linux Containers)
-   `/proc/self/mountinfo` (mount points)
-   `/proc/interrupts` (total and per core hardware interrupts)
-   `/proc/softirqs` (total and per core software interrupts)
-   `/proc/loadavg` (system load and total processes running)
-   `/proc/pressure/{cpu,memory,io}` (pressure stall information)
-   `/proc/sys/kernel/random/entropy_avail` (random numbers pool availability - used in cryptography)
-   `/proc/spl/kstat/zfs/arcstats` (status of ZFS adaptive replacement cache)
-   `/proc/spl/kstat/zfs/pool/state` (state of ZFS pools)
-   `/sys/class/power_supply` (power supply properties)
-   `/sys/class/infiniband` (infiniband interconnect)
-   `ipc` (IPC semaphores and message queues)
-   `ksm` Kernel Same-Page Merging performance (several files under `/sys/kernel/mm/ksm`).
-   `netdata` (internal Netdata resources utilization)

- - -

## Monitoring Disks

> Live demo of disk monitoring at: **[http://london.netdata.rocks](https://registry.my-netdata.io/#menu_disk)**

Performance monitoring for Linux disks is quite complicated. The main reason is the plethora of disk technologies available. There are many different hardware disk technologies, but there are even more **virtual disk** technologies that can provide additional storage features.

Hopefully, the Linux kernel provides many metrics that can provide deep insights of what our disks our doing. The kernel measures all these metrics on all layers of storage: **virtual disks**, **physical disks** and **partitions of disks**.

### Monitored disk metrics

-   **I/O bandwidth/s (kb/s)**
    The amount of data transferred from and to the disk.
-   **Amount of discarded data (kb/s)**
-   **I/O operations/s**
    The number of I/O operations completed.
-   **Extended I/O operations/s**
    The number of extended I/O operations completed.
-   **Queued I/O operations**
    The number of currently queued I/O operations. For traditional disks that execute commands one after another, one of them is being run by the disk and the rest are just waiting in a queue.
-   **Backlog size (time in ms)**
    The expected duration of the currently queued I/O operations.
-   **Utilization (time percentage)**
    The percentage of time the disk was busy with something. This is a very interesting metric, since for most disks, that execute commands sequentially, **this is the key indication of congestion**. A sequential disk that is 100% of the available time busy, has no time to do anything more, so even if the bandwidth or the number of operations executed by the disk is low, its capacity has been reached.
    Of course, for newer disk technologies (like fusion cards) that are capable to execute multiple commands in parallel, this metric is just meaningless.
-   **Average I/O operation time (ms)**
    The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
-   **Average I/O operation time for extended operations (ms)**
    The average time for extended I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
-   **Average I/O operation size (kb)**
    The average amount of data of the completed I/O operations.
-   **Average amount of discarded data (kb)**
    The average amount of data of the completed discard operations.
-   **Average Service Time (ms)**
    The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.
-   **Average Service Time for extended I/O operations (ms)**
    The average service time for completed extended I/O operations.
-   **Merged I/O operations/s**
    The Linux kernel is capable of merging I/O operations. So, if two requests to read data from the disk are adjacent, the Linux kernel may merge them to one before giving them to disk. This metric measures the number of operations that have been merged by the Linux kernel.
-   **Merged discard operations/s**
-   **Total I/O time**
    The sum of the duration of all completed I/O operations. This number can exceed the interval if the disk is able to execute multiple I/O operations in parallel.
-   **Space usage**
    For mounted disks, Netdata will provide a chart for their space, with 3 dimensions:
    1.  free
    2.  used
    3.  reserved for root
-   **inode usage**
    For mounted disks, Netdata will provide a chart for their inodes (number of file and directories), with 3 dimensions:
    1.  free
    2.  used
    3.  reserved for root

### disk names

Netdata will automatically set the name of disks on the dashboard, from the mount point they are mounted, of course only when they are mounted. Changes in mount points are not currently detected (you will have to restart Netdata to change the name of the disk). To use disk IDs provided by `/dev/disk/by-id`, the `name disks by id` option should be enabled. The `preferred disk ids` simple pattern allows choosing disk IDs to be used in the first place.

### performance metrics

By default, Netdata will enable monitoring metrics only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though). Set `yes` for a chart instead of `auto` to enable it permanently. You can also set the `enable zero metrics` option to `yes` in the `[global]` section which enables charts with zero metrics for all internal Netdata plugins.

Netdata categorizes all block devices in 3 categories:

1.  physical disks (i.e. block devices that do not have child devices and are not partitions)
2.  virtual disks (i.e. block devices that have child devices - like RAID devices)
3.  disk partitions (i.e. block devices that are part of a physical disk)

Performance metrics are enabled by default for all disk devices, except partitions and not-mounted virtual disks. Of course, you can enable/disable monitoring any block device by editing the Netdata configuration file.

### Netdata configuration

You can get the running Netdata configuration using this:

```sh
cd /etc/netdata
curl "http://localhost:19999/netdata.conf" >netdata.conf.new
mv netdata.conf.new netdata.conf
```

Then edit `netdata.conf` and find the following section. This is the basic plugin configuration.

```
[plugin:proc:/proc/diskstats]
  # enable new disks detected at runtime = yes
  # performance metrics for physical disks = auto
  # performance metrics for virtual disks = auto
  # performance metrics for partitions = no
  # bandwidth for all disks = auto
  # operations for all disks = auto
  # merged operations for all disks = auto
  # i/o time for all disks = auto
  # queued operations for all disks = auto
  # utilization percentage for all disks = auto
  # extended operations for all disks = auto
  # backlog for all disks = auto
  # bcache for all disks = auto
  # bcache priority stats update every = 0
  # remove charts of removed disks = yes
  # path to get block device = /sys/block/%s
  # path to get block device bcache = /sys/block/%s/bcache
  # path to get virtual block device = /sys/devices/virtual/block/%s
  # path to get block device infos = /sys/dev/block/%lu:%lu/%s
  # path to device mapper = /dev/mapper
  # path to /dev/disk/by-label = /dev/disk/by-label
  # path to /dev/disk/by-id = /dev/disk/by-id
  # path to /dev/vx/dsk = /dev/vx/dsk
  # name disks by id = no
  # preferred disk ids = *
  # exclude disks = loop* ram*
  # filename to monitor = /proc/diskstats
  # performance metrics for disks with major 8 = yes
```

For each virtual disk, physical disk and partition you will have a section like this:

```
[plugin:proc:/proc/diskstats:sda]
	# enable = yes
	# enable performance metrics = auto
	# bandwidth = auto
	# operations = auto
	# merged operations = auto
	# i/o time = auto
	# queued operations = auto
	# utilization percentage = auto
    # extended operations = auto
	# backlog = auto
```

For all configuration options:

-   `auto` = enable monitoring if the collected values are not zero
-   `yes` = enable monitoring
-   `no` = disable monitoring

Of course, to set options, you will have to uncomment them. The comments show the internal defaults.

After saving `/etc/netdata/netdata.conf`, restart your Netdata to apply them.

#### Disabling performance metrics for individual device and to multiple devices by device type

You can pretty easy disable performance metrics for individual device, for ex.:

```
[plugin:proc:/proc/diskstats:sda]
	enable performance metrics = no
```

But sometimes you need disable performance metrics for all devices with the same type, to do it you need to figure out device type from `/proc/diskstats` for ex.:

```
   7       0 loop0 1651 0 3452 168 0 0 0 0 0 8 168
   7       1 loop1 4955 0 11924 880 0 0 0 0 0 64 880
   7       2 loop2 36 0 216 4 0 0 0 0 0 4 4
   7       6 loop6 0 0 0 0 0 0 0 0 0 0 0
   7       7 loop7 0 0 0 0 0 0 0 0 0 0 0
 251       2 zram2 27487 0 219896 188 79953 0 639624 1640 0 1828 1828
 251       3 zram3 27348 0 218784 152 79952 0 639616 1960 0 2060 2104
```

All zram devices starts with `251` number and all loop devices starts with `7`.
So, to disable performance metrics for all loop devices you could add `performance metrics for disks with major 7 = no` to `[plugin:proc:/proc/diskstats]` section.

```
[plugin:proc:/proc/diskstats]
       performance metrics for disks with major 7 = no
```

## Monitoring RAID arrays

### Monitored RAID array metrics

1.  **Health** Number of failed disks in every array (aggregate chart).

2.  **Disks stats**

-   total (number of devices array ideally would have)
-   inuse (number of devices currently are in use)

3.  **Mismatch count**

-   unsynchronized blocks

4.  **Current status**

-   resync in percent
-   recovery in percent
-   reshape in percent
-   check in percent

5.  **Operation status** (if resync/recovery/reshape/check is active)

-   finish in minutes
-   speed in megabytes/s

6.  **Nonredundant array availability**

#### configuration

```
[plugin:proc:/proc/mdstat]
  # faulty devices = yes
  # nonredundant arrays availability = yes
  # mismatch count = auto
  # disk stats = yes
  # operation status = yes
  # make charts obsolete = yes
  # filename to monitor = /proc/mdstat
  # mismatch_cnt filename to monitor = /sys/block/%s/md/mismatch_cnt
```

## Monitoring CPUs

The `/proc/stat` module monitors CPU utilization, interrupts, context switches, processes started/running, thermal
throttling, frequency, and idle states. It gathers this information from multiple files.

If your system has more than 50 processors (`physical processors * cores per processor * threads per core`), the Agent
automatically disables CPU thermal throttling, frequency, and idle state charts. To override this default, see the next
section on configuration.

### Configuration

The settings for monitoring CPUs is in the `[plugin:proc:/proc/stat]` of your `netdata.conf` file.

The `keep per core files open` option lets you reduce the number of file operations on multiple files.

If your system has more than 50 processors and you would like to see the CPU thermal throttling, frequency, and idle
state charts that are automatically disabled, you can set the following boolean options in the
`[plugin:proc:/proc/stat]` section.

```conf
    keep per core files open = yes
    keep cpuidle files open = yes
    core_throttle_count = yes
    package_throttle_count = yes
    cpu frequency = yes
    cpu idle states = yes
```

### CPU frequency

The module shows the current CPU frequency as set by the `cpufreq` kernel
module.

**Requirement:**
You need to have `CONFIG_CPU_FREQ` and (optionally) `CONFIG_CPU_FREQ_STAT`
enabled in your kernel.

`cpufreq` interface provides two different ways of getting the information through `/sys/devices/system/cpu/cpu*/cpufreq/scaling_cur_freq` and `/sys/devices/system/cpu/cpu*/cpufreq/stats/time_in_state` files. The latter is more accurate so it is preferred in the module. `scaling_cur_freq` represents only the current CPU frequency, and doesn't account for any state changes which happen between updates. The module switches back and forth between these two methods if governor is changed.

It produces one chart with multiple lines (one line per core).

#### configuration

`scaling_cur_freq filename to monitor` and `time_in_state filename to monitor` in the `[plugin:proc:/proc/stat]` configuration section

### CPU idle states

The module monitors the usage of CPU idle states.

**Requirement:**
Your kernel needs to have `CONFIG_CPU_IDLE` enabled.

It produces one stacked chart per CPU, showing the percentage of time spent in
each state.

#### configuration

`schedstat filename to monitor`, `cpuidle name filename to monitor`, and `cpuidle time filename to monitor` in the `[plugin:proc:/proc/stat]` configuration section

## Monitoring memory

### Monitored memory metrics

-  Amount of memory swapped in/out
-  Amount of memory paged from/to disk
-  Number of memory page faults
-  Number of out of memory kills
-  Number of NUMA events

### Configuration

```conf
[plugin:proc:/proc/vmstat]
	filename to monitor = /proc/vmstat
	swap i/o = auto
	disk i/o = yes
	memory page faults = yes
	out of memory kills = yes
	system-wide numa metric summary = auto
```

## Monitoring Network Interfaces

### Monitored network interface metrics

-   **Physical Network Interfaces Aggregated Bandwidth (kilobits/s)**
    The amount of data received and sent through all physical interfaces in the system. This is the source of data for the Net Inbound and Net Outbound dials in the System Overview section.

-   **Bandwidth (kilobits/s)**
    The amount of data received and sent through the interface.

-   **Packets (packets/s)**
    The number of packets received, packets sent, and multicast packets transmitted through the interface.

-   **Interface Errors (errors/s)**
    The number of errors for the inbound and outbound traffic on the interface.

-   **Interface Drops (drops/s)**
    The number of packets dropped for the inbound and outbound traffic on the interface.

-   **Interface FIFO Buffer Errors (errors/s)**
    The number of FIFO buffer errors encountered while receiving and transmitting data through the interface.

-   **Compressed Packets (packets/s)**
    The number of compressed packets transmitted or received by the device driver.

-   **Network Interface Events (events/s)**
    The number of packet framing errors, collisions detected on the interface, and carrier losses detected by the device driver.

By default Netdata will enable monitoring metrics only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after Netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though).

### Monitoring wireless network interfaces

The settings for monitoring wireless is in the `[plugin:proc:/proc/net/wireless]` section of your `netdata.conf` file.

```conf
    status for all interfaces = yes
    quality for all interfaces = yes
    discarded packets for all interfaces = yes
    missed beacon for all interface = yes
```

You can set the following values for each configuration option:

-   `auto` = enable monitoring if the collected values are not zero
-   `yes` = enable monitoring
-   `no` = disable monitoring

#### Monitored wireless interface metrics

-   **Status**
    The current state of the interface. This is a device-dependent option.

-   **Link**    
    Overall quality of the link. 

-   **Level**
    Received signal strength (RSSI), which indicates how strong the received signal is.
    
-   **Noise**
    Background noise level.    
    
-   **Discarded packets**
    Discarded packets for: Number of packets received with a different NWID or ESSID (`nwid`), unable to decrypt (`crypt`), hardware was not able to properly re-assemble the link layer fragments (`frag`), packets failed to deliver (`retry`), and packets lost in relation with specific wireless operations (`misc`). 
    
-   **Missed beacon**    
     Number of periodic beacons from the cell or the access point the interface has missed.
     
#### Wireless configuration     

#### alarms

There are several alarms defined in `health.d/net.conf`.

The tricky ones are `inbound packets dropped` and `inbound packets dropped ratio`. They have quite a strict policy so that they warn users about possible issues. These alarms can be annoying for some network configurations. It is especially true for some bonding configurations if an interface is a child or a bonding interface itself. If it is expected to have a certain number of drops on an interface for a certain network configuration, a separate alarm with different triggering thresholds can be created or the existing one can be disabled for this specific interface. It can be done with the help of the [families](/health/REFERENCE.md#alarm-line-families) line in the alarm configuration. For example, if you want to disable the `inbound packets dropped` alarm for `eth0`, set `families: !eth0 *` in the alarm definition for `template: inbound_packets_dropped`.

#### configuration

Module configuration:

```
[plugin:proc:/proc/net/dev]
  # filename to monitor = /proc/net/dev
  # path to get virtual interfaces = /sys/devices/virtual/net/%s
  # path to get net device speed = /sys/class/net/%s/speed
  # enable new interfaces detected at runtime = auto
  # bandwidth for all interfaces = auto
  # packets for all interfaces = auto
  # errors for all interfaces = auto
  # drops for all interfaces = auto
  # fifo for all interfaces = auto
  # compressed packets for all interfaces = auto
  # frames, collisions, carrier counters for all interfaces = auto
  # disable by default interfaces matching = lo fireqos* *-ifb
  # refresh interface speed every seconds = 10
```

Per interface configuration:

```
[plugin:proc:/proc/net/dev:enp0s3]
  # enabled = yes
  # virtual = no
  # bandwidth = auto
  # packets = auto
  # errors = auto
  # drops = auto
  # fifo = auto
  # compressed = auto
  # events = auto
```

## Linux Anti-DDoS

![image6](https://cloud.githubusercontent.com/assets/2662304/14253733/53550b16-fa95-11e5-8d9d-4ed171df4735.gif)

---

SYNPROXY is a TCP SYN packets proxy. It can be used to protect any TCP server (like a web server) from SYN floods and similar DDos attacks.

SYNPROXY is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections.

The net effect of this, is that the real servers will not notice any change during the attack. The valid TCP connections will pass through and served, while the attack will be stopped at the firewall.

Netdata does not enable SYNPROXY. It just uses the SYNPROXY metrics exposed by your kernel, so you will first need to configure it. The hard way is to run iptables SYNPROXY commands directly on the console. An easier way is to use [FireHOL](https://firehol.org/), which, is a firewall manager for iptables. FireHOL can configure SYNPROXY using the following setup guides:

-   **[Working with SYNPROXY](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY)**
-   **[Working with SYNPROXY and traps](https://github.com/firehol/firehol/wiki/Working-with-SYNPROXY-and-traps)**

### Real-time monitoring of Linux Anti-DDoS

Netdata is able to monitor in real-time (per second updates) the operation of the Linux Anti-DDoS protection.

It visualizes 4 charts:

1.  TCP SYN Packets received on ports operated by SYNPROXY
2.  TCP Cookies (valid, invalid, retransmits)
3.  Connections Reopened
4.  Entries used

Example image:

![ddos](https://cloud.githubusercontent.com/assets/2662304/14398891/6016e3fc-fdf0-11e5-942b-55de6a52cb66.gif)

See Linux Anti-DDoS in action at: **[Netdata demo site (with SYNPROXY enabled)](https://registry.my-netdata.io/#menu_netfilter_submenu_synproxy)**

## Linux power supply

This module monitors various metrics reported by power supply drivers
on Linux. This allows tracking and alerting on things like remaining
battery capacity.

Depending on the underlying driver, it may provide the following charts
and metrics:

1.  Capacity: The power supply capacity expressed as a percentage.

    -   capacity_now

2.  Charge: The charge for the power supply, expressed as amphours.

    -   charge_full_design
    -   charge_full
    -   charge_now
    -   charge_empty
    -   charge_empty_design

3.  Energy: The energy for the power supply, expressed as watthours.

    -   energy_full_design
    -   energy_full
    -   energy_now
    -   energy_empty
    -   energy_empty_design

4.  Voltage: The voltage for the power supply, expressed as volts.

    -   voltage_max_design
    -   voltage_max
    -   voltage_now
    -   voltage_min
    -   voltage_min_design

#### configuration

```
[plugin:proc:/sys/class/power_supply]
  # battery capacity = yes
  # battery charge = no
  # battery energy = no
  # power supply voltage = no
  # keep files open = auto
  # directory to monitor = /sys/class/power_supply
```

#### notes

-   Most drivers provide at least the first chart. Battery powered ACPI
    compliant systems (like most laptops) provide all but the third, but do
    not provide all of the metrics for each chart.

-   Current, energy, and voltages are reported with a *very* high precision
    by the power_supply framework.  Usually, this is far higher than the
    actual hardware supports reporting, so expect to see changes in these
    charts jump instead of scaling smoothly.

-   If `max` or `full` attribute is defined by the driver, but not a
    corresponding `min` or `empty` attribute, then Netdata will still provide
    the corresponding `min` or `empty`, which will then always read as zero.
    This way, alerts which match on these will still work.

## Infiniband interconnect

This module monitors every active Infiniband port. It provides generic counters statistics, and per-vendor hw-counters (if vendor is supported).

### Monitored interface metrics

Each port will have its counters metrics monitored, grouped in the following charts:

-   **Bandwidth usage**
    Sent/Received data, in KB/s

-   **Packets Statistics**
    Sent/Received packets, in 3 categories: total, unicast and multicast.

-  **Errors Statistics**
    Many errors counters are provided, presenting statistics for:
    - Packets: malformated, sent/received discarded by card/switch, missing ressource
    - Link: downed, recovered, integrity error, minor error
    - Other events: Tick Wait to send, buffer overrun

If your vendor is supported, you'll also get HW-Counters statistics. These being vendor specific, please refer to their documentation.

- Mellanox: [see statistics documentation](https://community.mellanox.com/s/article/understanding-mlx5-linux-counters-and-status-parameters)

### configuration

Default configuration will monitor only enabled infiniband ports, and refresh newly activated or created ports every 30 seconds

```
[plugin:proc:/sys/class/infiniband]
  # dirname to monitor = /sys/class/infiniband
  # bandwidth counters = yes
  # packets counters = yes
  # errors counters = yes
  # hardware packets counters = auto
  # hardware errors counters = auto
  # monitor only ports being active = auto
  # disable by default interfaces matching = 
  # refresh ports state every seconds = 30
```


## IPC

### Monitored IPC metrics

-   **number of messages in message queues**
-   **amount of memory used by message queues**
-   **number of semaphores**
-   **number of semaphore arrays**
-   **number of shared memory segments**
-   **amount of memory used by shared memory segments**

As far as the message queue charts are dynamic, sane limits are applied for the number of dimensions per chart (the limit is configurable).

### configuration

```
[plugin:proc:ipc]
  # message queues = yes
  # semaphore totals = yes
  # shared memory totals = yes
  # msg filename to monitor = /proc/sysvipc/msg
  # shm filename to monitor = /proc/sysvipc/shm
  # max dimensions in memory allowed = 50
```

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fproc.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
