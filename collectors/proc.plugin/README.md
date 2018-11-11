# proc.plugin

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
 - `/sys/fs/cgroup` (Control Groups - Linux Containers)
 - `/proc/self/mountinfo` (mount points)
 - `/proc/interrupts` (total and per core hardware interrupts)
 - `/proc/softirqs` (total and per core software interrupts)
 - `/proc/loadavg` (system load and total processes running)
 - `/proc/sys/kernel/random/entropy_avail` (random numbers pool availability - used in cryptography)
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

## Proper reporting per-process CPU usage

**Console tools lie to us!**

Let's see an example. Execute this in a shell:

```sh
while [ 1 ]; do ls -l /var/run >/dev/null; done
```

In most systems `/var/run` is a `tmpfs` device, so there is nothing that can stop this command from consuming entirely one of the CPU cores of the machine.

As we will see below, **none** of the console performance monitoring tools can report that this command is using 100% CPU. They do report of course that the CPU is busy, but **they fail to identify the process that consumes so much CPU**.

Here is what common Linux console monitoring tools report:

### top

`top` reports that `bash` is using just 14%.

If you check the total system CPU utilization, it says there is no idle CPU at all, but `top` fails to provide a breakdown of the CPU consumption in the system. The sum of the CPU utilization of all processes reported by `top`, is 15.6%.

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

`atop` is also suffering from the same syndrome.

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

And the same is true for `glances`. The system runs at 100%, but `glances` reports only 17% per process utilization.

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

## why this happens?

All the console tools report usage based on the processes found running *at the moment they examine the process tree*. So, they see just one `ls` command, which is actually very quick with minor CPU utilization. But the shell, is spawning hundreds of them, one after another (much like shell scripts do).

When I realized this fact, I got surprised! The Linux kernel **accounts at the parent process, the CPU time of processes that exit**. However, the calculation to properly report the CPU time on each process, including its children that have exited, is quite tricky, so all console tools preferred to just ignore it!

## what netdata reports?

The total CPU utilization of the system:

![image](https://cloud.githubusercontent.com/assets/2662304/21076212/9198e5a6-bf2e-11e6-9bc0-6bdea25befb2.png)
<br/>_**Figure 1**: The system overview section at netdata, just a few seconds after the command was run_

And now, the applications netdata believes are using all this CPU:

![image](https://cloud.githubusercontent.com/assets/2662304/21076220/c9687848-bf2e-11e6-8d81-348592c5aca2.png)
<br/>_**Figure 2**: The Applications section at netdata, just a few seconds after the command was run_

So, my `ssh` session is using 95% CPU time.

Why `ssh`?

`apps.plugin` groups all processes based on its configuration file ([`/etc/netdata/apps_groups.conf`](https://github.com/netdata/netdata/blob/master/conf.d/apps_groups.conf)). The default configuration has nothing for `bash`, but it has for `sshd`, so netdata accumulates all ssh sessions to a dimension on the charts, called `ssh`. This includes all the processes in the process tree of `sshd`, **including the exited children**.

> Distributions based on `systemd`, provide another way to get cpu utilization per user session or service running: control groups, or cgroups, commonly used as part of containers. `apps.plugin` does not use these mechanisms. The process grouping made by `apps.plugin` works on any Linux, `systemd` based or not.
> Keep in mind **netdata** does detect containers and report resource utilization for each of them, but this is subject to another article...

### a more technical description of how netdata works

netdata reads `/proc/<pid>/stat` for all processes, once per second and extracts `utime` and `stime` (user and system cpu utilization), much like all the console tools do.

But it [also extracts `cutime` and `cstime`](https://github.com/netdata/netdata/blob/62596cc6b906b1564657510ca9135c08f6d4cdda/src/apps_plugin.c#L636-L642) that account the user and system time of the exited children of each process. By keeping a map in memory of the whole process tree, it is capable of assigning the right time to every process, taking into account all its exited children.

It is tricky, since a process may be running for 1 hour and once it exits, its parent should not receive the whole 1 hour of cpu time in just 1 second - you have to subtract the cpu time that has been reported for it prior to this iteration.

It is even trickier, because walking through the entire process tree takes some time itself. So, if you sum the CPU utilization of all processes, you might have more CPU time than the reported total cpu time of the system. netdata solves this, by adapting the per process cpu utilization to the total of the system. [Netdata adds charts that document this normalization](https://london.my-netdata.io/default.html#menu_netdata_submenu_apps_plugin).

### Conclusion

From a performance monitoring perspective, the lack of properly identifying the CPU consumption used by processes, is terrifying. All console tools I found, fail to properly report where the CPU goes.

In several cases, this leads to wrong conclusions. For example, if you use **pacemaker** clusters you may not be aware that the clustering software itself requires so much CPU.

pacemaker running on server A:

![image](https://cloud.githubusercontent.com/assets/2662304/21076003/3ebf7fac-bf29-11e6-9953-272518a15876.png)

`top` reports only 2% for `lrmd`, while it consumes 35% on the average, with spikes even above 100% (100% = 1 core).

Similarly, pacemaker running on server B:

![image](https://cloud.githubusercontent.com/assets/2662304/21076040/2870d3b2-bf2a-11e6-9fa8-eee228be116e.png)

