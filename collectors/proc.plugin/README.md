# proc.plugin

 - `/proc/net/dev` (all network interfaces for all their values)
 - `/proc/diskstats` (all disks for all their values)
 - `/proc/mdstat` (status of RAID arrays)
 - `/proc/net/snmp` (total IPv4, TCP and UDP usage)
 - `/proc/net/snmp6` (total IPv6 usage)
 - `/proc/net/netstat` (more IPv4 usage)
 - `/proc/net/stat/nf_conntrack` (connection tracking performance)
 - `/proc/net/stat/synproxy` (synproxy performance)
 - `/proc/net/ip_vs/stats` (IPVS connection statistics)
 - `/proc/stat` (CPU utilization and attributes)
 - `/proc/meminfo` (memory information)
 - `/proc/vmstat` (system performance)
 - `/proc/net/rpc/nfsd` (NFS server statistics for both v3 and v4 NFS servers)
 - `/sys/fs/cgroup` (Control Groups - Linux Containers)
 - `/proc/self/mountinfo` (mount points)
 - `/proc/interrupts` (total and per core hardware interrupts)
 - `/proc/softirqs` (total and per core software interrupts)
 - `/proc/loadavg` (system load and total processes running)
 - `/proc/sys/kernel/random/entropy_avail` (random numbers pool availability - used in cryptography)
 - `/sys/class/power_supply` (power supply properties)
 - `ksm` Kernel Same-Page Merging performance (several files under `/sys/kernel/mm/ksm`).
 - `netdata` (internal netdata resources utilization)


---

## Monitoring Disks

> Live demo of disk monitoring at: **[http://london.netdata.rocks](https://registry.my-netdata.io/#menu_disk)**

Performance monitoring for Linux disks is quite complicated. The main reason is the plethora of disk technologies available. There are many different hardware disk technologies, but there are even more **virtual disk** technologies that can provide additional storage features.

Hopefully, the Linux kernel provides many metrics that can provide deep insights of what our disks our doing. The kernel measures all these metrics on all layers of storage: **virtual disks**, **physical disks** and **partitions of disks**.

### Monitored disk metrics

- I/O bandwidth/s (kb/s)
  The amount of data transferred from and to the disk.
- I/O operations/s
  The number of I/O operations completed.
- Queued I/O operations
  The number of currently queued I/O operations. For traditional disks that execute commands one after another, one of them is being run by the disk and the rest are just waiting in a queue.
- Backlog size (time in ms)
  The expected duration of the currently queued I/O operations.
- Utilization (time percentage)
  The percentage of time the disk was busy with something. This is a very interesting metric, since for most disks, that execute commands sequentially, **this is the key indication of congestion**. A sequential disk that is 100% of the available time busy, has no time to do anything more, so even if the bandwidth or the number of operations executed by the disk is low, its capacity has been reached.
  Of course, for newer disk technologies (like fusion cards) that are capable to execute multiple commands in parallel, this metric is just meaningless.
- Average I/O operation time (ms)
  The average time for I/O requests issued to the device to be served. This includes the time spent by the requests in queue and the time spent servicing them.
- Average I/O operation size (kb)
  The average amount of data of the completed I/O operations.
- Average Service Time (ms)
  The average service time for completed I/O operations. This metric is calculated using the total busy time of the disk and the number of completed operations. If the disk is able to execute multiple parallel operations the reporting average service time will be misleading.
- Merged I/O operations/s
  The Linux kernel is capable of merging I/O operations. So, if two requests to read data from the disk are adjacent, the Linux kernel may merge them to one before giving them to disk. This metric measures the number of operations that have been merged by the Linux kernel.
- Total I/O time
  The sum of the duration of all completed I/O operations. This number can exceed the interval if the disk is able to execute multiple I/O operations in parallel.
- Space usage
  For mounted disks, netdata will provide a chart for their space, with 3 dimensions:
  1. free
  2. used
  3. reserved for root
- inode usage
  For mounted disks, netdata will provide a chart for their inodes (number of file and directories), with 3 dimensions:
  1. free
  2. used
  3. reserved for root

### disk names

netdata will automatically set the name of disks on the dashboard, from the mount point they are mounted, of course only when they are mounted. Changes in mount points are not currently detected (you will have to restart netdata to change the name of the disk).

### performance metrics

By default netdata will enable monitoring metrics only when they are not zero. If they are constantly zero they are ignored. Metrics that will start having values, after netdata is started, will be detected and charts will be automatically added to the dashboard (a refresh of the dashboard is needed for them to appear though).

netdata categorizes all block devices in 3 categories:

1. physical disks (i.e. block devices that does not have slaves and are not partitions)
2. virtual disks (i.e. block devices that have slaves - like RAID devices)
3. disk partitions (i.e. block devices that are part of a physical disk)

Performance metrics are enabled by default for all disk devices, except partitions and not-mounted virtual disks. Of course, you can enable/disable monitoring any block device by editing the netdata configuration file.

### netdata configuration

You can get the running netdata configuration using this:

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
	# performance metrics for virtual disks = no
	# performance metrics for partitions = no
	# performance metrics for mounted filesystems = no
	# performance metrics for mounted virtual disks = auto
	# space metrics for mounted filesystems = auto
	# bandwidth for all disks = auto
	# operations for all disks = auto
	# merged operations for all disks = auto
	# i/o time for all disks = auto
	# queued operations for all disks = auto
	# utilization percentage for all disks = auto
	# backlog for all disks = auto
	# space usage for all disks = auto
	# inodes usage for all disks = auto
	# filename to monitor = /proc/diskstats
	# path to get block device infos = /sys/dev/block/%lu:%lu/%s
	# path to get h/w sector size = /sys/block/%s/queue/hw_sector_size
	# path to get h/w sector size for partitions = /sys/dev/block/%lu:%lu/subsystem/%s/../queue
/hw_sector_size

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
	# backlog = auto
```

For all configuration options:
- `auto` = enable monitoring if the collected values are not zero
- `yes` = enable monitoring
- `no` = disable monitoring

Of course, to set options, you will have to uncomment them. The comments show the internal defaults.

After saving `/etc/netdata/netdata.conf`, restart your netdata to apply them.

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

1. **Health** Number of failed disks in every array (aggregate chart).

2. **Disks stats**
 * total (number of devices array ideally would have)
 * inuse (number of devices currently are in use)

3. **Mismatch count**
 * unsynchronized blocks

4. **Current status**
 * resync in percent
 * recovery in percent
 * reshape in percent
 * check in percent

5. **Operation status** (if resync/recovery/reshape/check is active)
 * finish in minutes
 * speed in megabytes/s

6. **Nonredundant array availability**

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

The `/proc/stat` module monitors CPU utilization, interrupts, context switches, processes started/running, thermal throttling, frequency, and idle states. It gathers this information from multiple files.

If more than 50 cores are present in a system then CPU thermal throttling, frequency, and idle state charts are disabled.

#### configuration

`keep per core files open` option in the `[plugin:proc:/proc/stat]` configuration section allows reducing the number of file operations on multiple files.

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

## Linux Anti-DDoS

![image6](https://cloud.githubusercontent.com/assets/2662304/14253733/53550b16-fa95-11e5-8d9d-4ed171df4735.gif)

---
SYNPROXY is a TCP SYN packets proxy. It can be used to protect any TCP server (like a web server) from SYN floods and similar DDos attacks.

SYNPROXY is a netfilter module, in the Linux kernel (since version 3.12). It is optimized to handle millions of packets per second utilizing all CPUs available without any concurrency locking between the connections.

The net effect of this, is that the real servers will not notice any change during the attack. The valid TCP connections will pass through and served, while the attack will be stopped at the firewall.

Netdata does not enable SYNPROXY. It just uses the SYNPROXY metrics exposed by your kernel, so you will first need to configure it. The hard way is to run iptables SYNPROXY commands directly on the console. An easier way is to use [FireHOL](https://firehol.org/), which, is a firewall manager for iptables. FireHOL can configure SYNPROXY using the following setup guides:

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

See Linux Anti-DDoS in action at: **[netdata demo site (with SYNPROXY enabled)](https://registry.my-netdata.io/#menu_netfilter_submenu_synproxy)**

## Linux power supply

This module monitors various metrics reported by power supply drivers
on Linux. This allows tracking and alerting on things like remaining
battery capacity.

Depending on the underlying driver, it may provide the following charts
and metrics:

1. Capacity: The power supply capacity expressed as a percentage.
  * capacity\_now

2. Charge: The charge for the power supply, expressed as amphours.
  * charge\_full\_design
  * charge\_full
  * charge\_now
  * charge\_empty
  * charge\_empty\_design

3. Energy: The energy for the power supply, expressed as watthours.
  * energy\_full\_design
  * energy\_full
  * energy\_now
  * energy\_empty
  * energy\_empty\_design

2. Voltage: The voltage for the power supply, expressed as volts.
  * voltage\_max\_design
  * voltage\_max
  * voltage\_now
  * voltage\_min
  * voltage\_min\_design

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

* Most drivers provide at least the first chart. Battery powered ACPI
compliant systems (like most laptops) provide all but the third, but do
not provide all of the metrics for each chart.

* Current, energy, and voltages are reported with a _very_ high precision
by the power\_supply framework.  Usually, this is far higher than the
actual hardware supports reporting, so expect to see changes in these
charts jump instead of scaling smoothly.

* If `max` or `full` attribute is defined by the driver, but not a
corresponding `min` or `empty` attribute, then Netdata will still provide
the corresponding `min` or `empty`, which will then always read as zero.
This way, alerts which match on these will still work.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fproc.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
