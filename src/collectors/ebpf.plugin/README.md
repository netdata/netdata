<!--
title: "Kernel traces/metrics (eBPF) monitoring with Netdata"
description: "Use Netdata's extended Berkeley Packet Filter (eBPF) collector to monitor kernel-level metrics about yourcomplex applications with per-second granularity."
custom_edit_url: "https://github.com/netdata/netdata/edit/master/src/collectors/ebpf.plugin/README.md"
sidebar_label: "Kernel traces/metrics (eBPF)"
learn_status: "Published"
learn_topic_type: "References"
learn_rel_path: "Integrations/Monitor/System metrics"
-->

# Kernel traces/metrics (eBPF) collector

The Netdata Agent provides many [eBPF](https://ebpf.io/what-is-ebpf/) programs to help you troubleshoot and debug how applications interact with the Linux kernel. The `ebpf.plugin` uses [tracepoints, trampoline, and2 kprobes](#how-netdata-collects-data-using-probes-and-tracepoints) to collect a wide array of high value data about the host that would otherwise be impossible to capture. 

> ❗ eBPF monitoring only works on Linux systems and with specific Linux kernels, including all kernels newer than `4.11.0`, and all kernels on CentOS 7.6 or later. For kernels older than `4.11.0`, improved support is in active development.

This document provides comprehensive details about the `ebpf.plugin`.
For hands-on configuration and troubleshooting tips see our [tutorial on troubleshooting apps with eBPF metrics](https://github.com/netdata/netdata/blob/master/docs/developer-and-contributor-corner/monitor-debug-applications-ebpf.md).

<figure>
  <img src="https://user-images.githubusercontent.com/1153921/74746434-ad6a1e00-5222-11ea-858a-a7882617ae02.png" alt="An example of VFS charts, made possible by the eBPF collector plugin" />
  <figcaption>An example of virtual file system (VFS) charts made possible by the eBPF collector plugin.</figcaption>
</figure>

## How Netdata collects data using probes and tracepoints

Netdata uses the following features from the Linux kernel to run eBPF programs:

-   Tracepoints are hooks to call specific functions. Tracepoints are more stable than `kprobes` and are preferred when
    both options are available.
-   Trampolines are bridges between kernel functions, and BPF programs. Netdata uses them by default whenever available.
-   Kprobes and return probes (`kretprobe`): Probes can insert virtually into any kernel instruction. When eBPF runs in `entry` mode, it attaches only `kprobes` for internal functions monitoring calls and some arguments every time a function is called. The user can also change configuration to use [`return`](#global-configuration-options) mode, and this will allow users to monitor return from these functions and detect possible failures.

In each case, wherever a normal kprobe, kretprobe, or tracepoint would have run its hook function, an eBPF program is run instead, performing various collection logic before letting the kernel continue its normal control flow.

There are more methods to trigger eBPF programs, such as uprobes, but currently are not supported.

## Configuring ebpf.plugin

The eBPF collector is installed and enabled by default on most new installations of the Agent.
If your Agent is v1.22 or older, you may to enable the collector yourself. 

### Enable the eBPF collector

To enable or disable the entire eBPF collector: 

1.  Navigate to the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).
    ```bash
    cd /etc/netdata
    ```

2.  Use the [`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) script to edit `netdata.conf`.

    ```bash
    ./edit-config netdata.conf
    ```

3.  Enable the collector by scrolling down to the `[plugins]` section. Uncomment the line `ebpf` (not
    `ebpf_process`) and set it to `yes`.

    ```conf
    [plugins]
       ebpf = yes
    ```

### Configure the eBPF collector

You can configure the eBPF collector's behavior to fine-tune which metrics you receive and [optimize performance]\(#performance opimization).

To edit the `ebpf.d.conf`:

1.  Navigate to the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).
    ```bash
    cd /etc/netdata
    ```
2.  Use the [`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) script to edit [`ebpf.d.conf`](https://github.com/netdata/netdata/blob/master/src/collectors/ebpf.plugin/ebpf.d.conf).

    ```bash
    ./edit-config ebpf.d.conf
    ```

    You can now edit the behavior of the eBPF collector. The following sections describe each configuration option in detail.

### `[global]` configuration options

The `[global]` section defines settings for the whole eBPF collector.

#### eBPF load mode

The collector uses two different eBPF programs. These programs rely on the same functions inside the kernel, but they
monitor, process, and display different kinds of information.

By default, this plugin uses the `entry` mode. Changing this mode can create significant overhead on your operating
system, but also offer valuable information if you are developing or debugging software. The `ebpf load mode` option
accepts the following values:

-   `entry`: This is the default mode. In this mode, the eBPF collector only monitors calls for the functions described in
    the sections above, and does not show charts related to errors.
-   `return`: In the `return` mode, the eBPF collector monitors the same kernel functions as `entry`, but also creates new
    charts for the return of these functions, such as errors. Monitoring function returns can help in debugging software,
    such as failing to close file descriptors or creating zombie processes.

#### Integration with `apps.plugin`

The eBPF collector also creates charts for each running application through an integration with the
[`apps.plugin`](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/README.md). This integration helps you understand how specific applications
interact with the Linux kernel.

If you want to enable `apps.plugin` integration, change the "apps" setting to "yes".

```conf
[global]
   apps = yes
```

#### Integration with `cgroups.plugin`

The eBPF collector also creates charts for each cgroup through an integration with the
[`cgroups.plugin`](https://github.com/netdata/netdata/blob/master/src/collectors/cgroups.plugin/README.md). This integration helps you understand how a specific cgroup
interacts with the Linux kernel.

The integration with `cgroups.plugin` is disabled by default to avoid creating overhead on your system. If you want to
_enable_ the integration with `cgroups.plugin`, change the `cgroups` setting to `yes`.

```conf
[global]
   cgroups = yes
```

If you do not need to monitor specific metrics for your `cgroups`, you can enable `cgroups` inside
`ebpf.d.conf`, and then disable the plugin for a specific `thread` by following the steps in the
[Configuration](#configuring-ebpfplugin) section.

#### Maps per Core

When netdata is running on kernels newer than `4.6` users are allowed to modify how the `ebpf.plugin` creates maps (hash or 
array). When `maps per core` is defined as `yes`, plugin will create a map per core on host, on the other hand,
when the value is set as `no` only one hash table will be created, this option will use less memory, but it also can
increase overhead for processes.

#### Collect PID

When one of the previous integrations is enabled, `ebpf.plugin` will use Process Identifier (`PID`) to identify the
process group for which it needs to plot data.

There are different ways to collect PID, and you can select the way `ebpf.plugin` collects data with the following
values:

-   `real parent`: This is the default mode. Collection will aggregate data for the real parent, the thread that creates
     child threads.
-   `parent`: Parent and real parent are the same when a process starts, but this value can be changed during run time.
-   `all`: This option will store all PIDs that run on the host. Note, this method can be expensive for the host,
    because more memory needs to be allocated and parsed.

The threads that have integration with other collectors have an internal clean up wherein they attach either a
`trampoline` or a `kprobe` to `release_task` internal function. To avoid `overload` on this function, `ebpf.plugin`
will only enable these threads integrated with other collectors when the kernel is compiled with
`CONFIG_DEBUG_INFO_BTF`, unless you enable them manually.

#### Collection period

The plugin uses the option `update every` to define the number of seconds used for eBPF to send data for Netdata. The default value
is 5 seconds.

#### PID table size

The option `pid table size` defines the maximum number of PIDs stored inside the application hash table. The default value
is defined according [kernel](https://elixir.bootlin.com/linux/v6.0.19/source/include/linux/threads.h#L28) source code.

#### Integration Dashboard Elements

When an integration is enabled, your dashboard will also show the following cgroups and apps charts using low-level
Linux metrics:

> Note: The parenthetical accompanying each bulleted item provides the chart name.

-   mem
    -   Number of processes killed due out of memory. (`oomkills`)
-   process
    -   Number of processes created with `do_fork`. (`process_create`)
    -   Number of threads created with `do_fork` or `clone (2)`, depending on your system's kernel
        version. (`thread_create`)
    -   Number of times that a process called `do_exit`. (`task_exit`)
    -   Number of times that a process called `release_task`. (`task_close`)
    -   Number of times that an error happened to create thread or process. (`task_error`)
-   swap
    -   Number of calls to `swap_readpage`. (`swap_read_call`)
    -   Number of calls to `swap_writepage`. (`swap_write_call`)
-   network
    -   Number of outbound connections using TCP/IPv4. (`outbound_conn_ipv4`)
    -   Number of outbound connections using TCP/IPv6. (`outbound_conn_ipv6`)
    -   Number of bytes sent. (`total_bandwidth_sent`)
    -   Number of bytes received. (`total_bandwidth_recv`)
    -   Number of calls to `tcp_sendmsg`. (`bandwidth_tcp_send`)
    -   Number of calls to `tcp_cleanup_rbuf`. (`bandwidth_tcp_recv`)
    -   Number of calls to `tcp_retransmit_skb`. (`bandwidth_tcp_retransmit`)
    -   Number of calls to `udp_sendmsg`. (`bandwidth_udp_send`)
    -   Number of calls to `udp_recvmsg`. (`bandwidth_udp_recv`)
-   file access
    -   Number of calls to open files. (`file_open`)
    -   Number of calls to open files that returned errors. (`open_error`)
    -   Number of files closed. (`file_closed`)
    -   Number of calls to close files that returned errors. (`file_error_closed`)
-   vfs
    -   Number of calls to `vfs_unlink`. (`file_deleted`)
    -   Number of calls to `vfs_write`. (`vfs_write_call`)
    -   Number of calls to write a file that returned errors. (`vfs_write_error`)
    -   Number of calls to `vfs_read`. (`vfs_read_call`)
    -   -   Number of calls to read a file that returned errors. (`vfs_read_error`)
    -   Number of bytes written with `vfs_write`. (`vfs_write_bytes`)
    -   Number of bytes read with `vfs_read`. (`vfs_read_bytes`)
    -   Number of calls to `vfs_fsync`. (`vfs_fsync`)
    -   Number of calls to sync file that returned errors. (`vfs_fsync_error`)
    -   Number of calls to `vfs_open`. (`vfs_open`)
    -   Number of calls to open file that returned errors. (`vfs_open_error`)
    -   Number of calls to `vfs_create`. (`vfs_create`)
    -   Number of calls to open file that returned errors. (`vfs_create_error`)
-   page cache
    -   Ratio of pages accessed. (`cachestat_ratio`)
    -   Number of modified pages ("dirty"). (`cachestat_dirties`)
    -   Number of accessed pages. (`cachestat_hits`)
    -   Number of pages brought from disk. (`cachestat_misses`)
-   directory cache
    -   Ratio of files available in directory cache. (`dc_hit_ratio`)
    -   Number of files accessed. (`dc_reference`)
    -   Number of files accessed that were not in cache. (`dc_not_cache`)
    -   Number of files not found. (`dc_not_found`)
-   ipc shm
    -   Number of calls to `shm_get`. (`shmget_call`)
    -   Number of calls to `shm_at`. (`shmat_call`)
    -   Number of calls to `shm_dt`. (`shmdt_call`)
    -   Number of calls to `shm_ctl`. (`shmctl_call`)

### `[ebpf programs]` configuration options

The eBPF collector enables and runs the following eBPF programs by default:

-   `cachestat`: Netdata's eBPF data collector creates charts about the memory page cache. When the integration with
    [`apps.plugin`](https://github.com/netdata/netdata/blob/master/src/collectors/apps.plugin/README.md) is enabled, this collector creates charts for the whole host _and_
    for each application.
-   `fd` :  This eBPF program creates charts that show information about calls to open files.
-   `mount`: This eBPF program creates charts that show calls to syscalls mount(2) and umount(2).
-   `shm`: This eBPF program creates charts that show calls to syscalls shmget(2), shmat(2), shmdt(2) and shmctl(2).
-   `process`: This eBPF program creates charts that show information about process life. When in `return` mode, it also
    creates charts showing errors when these operations are executed.
-   `hardirq`: This eBPF program creates charts that show information about time spent servicing individual hardware
    interrupt requests (hard IRQs).
-   `softirq`: This eBPF program creates charts that show information about time spent servicing individual software
    interrupt requests (soft IRQs).
-   `oomkill`: This eBPF program creates a chart that shows OOM kills for all applications recognized via
    the `apps.plugin` integration. Note that this program will show application charts regardless of whether apps
    integration is turned on or off.

You can also enable the following eBPF programs:

-   `dcstat` : This eBPF program creates charts that show information about file access using directory cache. It appends
    `kprobes` for `lookup_fast()` and `d_lookup()` to identify if files are inside directory cache, outside and files are
    not found.
-   `disk` : This eBPF program creates charts that show information about disk latency independent of filesystem.
-   `filesystem` : This eBPF program creates charts that show information about some filesystem latency.
-   `swap` : This eBPF program creates charts that show information about swap access.
-   `mdflush`: This eBPF program creates charts that show information about
-   `sync`: Monitor calls to syscalls sync(2), fsync(2), fdatasync(2), syncfs(2), msync(2), and sync_file_range(2).
-   `socket`: This eBPF program creates charts with information about `TCP` and `UDP` functions, including the
    bandwidth consumed by each.
    multi-device software flushes.
-   `vfs`: This eBPF program creates charts that show information about VFS (Virtual File System) functions.

### Configuring eBPF threads

You can configure each thread of the eBPF data collector. This allows you to overwrite global options defined in `/etc/netdata/ebpf.d.conf` and configure specific options for each thread.

To configure an eBPF thread:

1.  Navigate to the [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory).
    ```bash
    cd /etc/netdata
    ```
2.  Use the [`edit-config`](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) script to edit a thread configuration file. The following configuration files are available:

    -   `network.conf`: Configuration for the [`network` thread](#network-configuration). This config file overwrites the global options and also
        lets you specify which network the eBPF collector monitors.
    -   `process.conf`: Configuration for the [`process` thread](#sync-configuration).
    -   `cachestat.conf`: Configuration for the `cachestat` thread(#filesystem-configuration).
    -   `dcstat.conf`: Configuration for the `dcstat` thread.
    -   `disk.conf`: Configuration for the `disk` thread.
    -   `fd.conf`: Configuration for the `file descriptor` thread.
    -   `filesystem.conf`: Configuration for the `filesystem` thread.
    -   `hardirq.conf`: Configuration for the `hardirq` thread.
    -   `softirq.conf`: Configuration for the `softirq` thread.
    -   `sync.conf`: Configuration for the `sync` thread.
    -   `vfs.conf`: Configuration for the `vfs` thread.

        ```bash
        ./edit-config FILE.conf
        ```

### Network configuration

The network configuration has specific options to configure which network(s) the eBPF collector monitors. These options
are divided in the following sections:

#### `[network connections]`

You can configure the information shown with function `ebpf_socket` using the settings in this section.

```conf
[network connections]
    enabled = yes
    resolve hostname ips = no
    resolve service names = yes
    ports = 1-1024 !145 !domain
    hostnames = !example.com
    ips = !127.0.0.1/8 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7
```

When you define a `ports` setting, Netdata will collect network metrics for that specific port. For example, if you
write `ports = 19999`, Netdata will collect only connections for itself. The `hostnames` setting accepts
[simple patterns](https://github.com/netdata/netdata/blob/master/src/libnetdata/simple_pattern/README.md). The `ports`, and `ips` settings accept negation (`!`) to deny
specific values or asterisk alone to define all values.

In the above example, Netdata will collect metrics for all ports between `1` and `1024`, with the exception of `53` (domain)
and `145`.

The following options are available:

-   `enabled`: Disable network connections monitoring. This can affect directly some funcion output.
-   `resolve hostname ips`: Enable resolving IPs to hostnames. It is disabled by default because it can be too slow.
-   `resolve service names`: Convert destination ports into service names, for example, port `53` protocol `UDP` becomes `domain`.
    all names are read from /etc/services.
-   `ports`: Define the destination ports for Netdata to monitor.
-   `hostnames`: The list of hostnames that can be resolved to an IP address.
-   `ips`: The IP or range of IPs that you want to monitor. You can use IPv4 or IPv6 addresses, use dashes to define a
    range of IPs, or use CIDR values.

By default the traffic table is created using the destination IPs and ports of the sockets. This can be
changed, so that Netdata uses service names (if possible), by specifying `resolve service name = yes` in the configuration
section.

#### `[service name]`

Netdata uses the list of services in `/etc/services` to plot network connection charts. If this file does not contain
the name for a particular service you use in your infrastructure, you will need to add it to the `[service name]`
section.

For example, Netdata's default port (`19999`) is not listed in `/etc/services`. To associate that port with the Netdata
service in network connection charts, and thus see the name of the service instead of its port, define it:

```conf
[service name]
    19999 = Netdata
```

### Sync configuration

The sync configuration has specific options to disable monitoring for syscalls.  All syscalls are monitored by default.

```conf
[syscalls]
    sync = yes
    msync = yes
    fsync = yes
    fdatasync = yes
    syncfs = yes
    sync_file_range = yes
```

### Filesystem configuration

The filesystem configuration has specific options to disable monitoring for filesystems; by default, all filesystems are
monitored.

```conf
[filesystem]
    btrfsdist = yes
    ext4dist = yes
    nfsdist = yes
    xfsdist = yes
    zfsdist = yes
```

The ebpf program `nfsdist` monitors only `nfs` mount points.

## Troubleshooting

If the eBPF collector does not work, you can troubleshoot it by running the `ebpf.plugin` command and investigating its
output.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo su -s /bin/bash ./ebpf.plugin
```

You can also use `grep` to search the Agent's `error.log` for messages related to eBPF monitoring.

```bash
grep -i ebpf /var/log/netdata/error.log
```

### Confirm kernel compatibility

The eBPF collector only works on Linux systems and with specific Linux kernels. We support all kernels more recent than
`4.11.0`, and all kernels on CentOS 7.6 or later.

You can run our helper script to determine whether your system can support eBPF monitoring. If it returns no output, your system is ready to compile and run the eBPF collector.

```bash
curl -sSL https://raw.githubusercontent.com/netdata/kernel-collector/master/tools/check-kernel-config.sh | sudo bash
```


If you see a warning about a missing kernel
configuration (`KPROBES KPROBES_ON_FTRACE HAVE_KPROBES BPF BPF_SYSCALL BPF_JIT`), you will need to recompile your kernel
to support this configuration. The process of recompiling Linux kernels varies based on your distribution and version.
Read the documentation for your system's distribution to learn more about the specific workflow for recompiling the
kernel, ensuring that you set all the necessary

-   [Ubuntu](https://wiki.ubuntu.com/Kernel/BuildYourOwnKernel)
-   [Debian](https://kernel-team.pages.debian.net/kernel-handbook/ch-common-tasks.html#s-common-official)
-   [Fedora](https://fedoraproject.org/wiki/Building_a_custom_kernel)
-   [CentOS](https://wiki.centos.org/HowTos/Custom_Kernel)
-   [Arch Linux](https://wiki.archlinux.org/index.php/Kernel/Traditional_compilation)
-   [Slackware](https://docs.slackware.com/howtos:slackware_admin:kernelbuilding)

### Mount `debugfs` and `tracefs`

The eBPF collector also requires both the `tracefs` and `debugfs` filesystems. Try mounting the `tracefs` and `debugfs`
filesystems using the commands below:

```bash
sudo mount -t debugfs nodev /sys/kernel/debug
sudo mount -t tracefs nodev /sys/kernel/tracing
```

If they are already mounted, you will see an error. You can also configure your system's `/etc/fstab` configuration to
mount these filesystems on startup. More information can be found in
the [ftrace documentation](https://www.kernel.org/doc/Documentation/trace/ftrace.txt).

## Charts

The eBPF collector creates charts on different menus, like System Overview, Memory, MD arrays, Disks, Filesystem,
Mount Points, Networking Stack, systemd Services, and Applications.

The collector stores the actual value inside of its process, but charts only show the difference between the values
collected in the previous and current seconds.

### System overview

Not all charts within the System Overview menu are enabled by default. Charts that rely on `kprobes` are disabled by default because they add around 100ns overhead for each function call. This is a small number from a human's perspective, but the functions are called many times and create an impact
on host. See the [configuration](#configuring-ebpfplugin) section for details about how to enable them.

#### Processes

Internally, the Linux kernel treats both processes and threads as `tasks`. To create a thread, the kernel offers a few
system calls: `fork(2)`, `vfork(2)`, and `clone(2)`. To generate this chart, the eBPF
collector uses the following `tracepoints` and `kprobe`:

-   `sched/sched_process_fork`: Tracepoint called after a call for `fork (2)`, `vfork (2)` and `clone (2)`.
-   `sched/sched_process_exec`: Tracepoint called after a exec-family syscall.
-   `kprobe/kernel_clone`: This is the main [`fork()`](https://elixir.bootlin.com/linux/v5.10/source/kernel/fork.c#L2415)
     routine since kernel `5.10.0` was released.
-   `kprobe/_do_fork`: Like `kernel_clone`, but this was the main function between kernels `4.2.0` and `5.9.16`
-   `kprobe/do_fork`: This was the main function before kernel `4.2.0`.

#### Process Exit

Ending a task requires two steps. The first is a call to the internal function `do_exit`, which notifies the operating
system that the task is finishing its work. The second step is to release the kernel information with the internal
function `release_task`. The difference between the two dimensions can help you discover
[zombie processes](https://en.wikipedia.org/wiki/Zombie_process). To get the metrics, the collector uses:

-   `sched/sched_process_exit`: Tracepoint called after a task exits.
-   `kprobe/release_task`: This function is called when a process exits, as the kernel still needs to remove the process
    descriptor.

#### Task error

The functions responsible for ending tasks do not return values, so this chart contains information about failures on
process and thread creation only.

#### Swap

Inside the swap submenu the eBPF plugin creates the chart `swapcalls`; this chart is displaying when processes are
calling functions [`swap_readpage` and `swap_writepage`](https://hzliu123.github.io/linux-kernel/Page%20Cache%20in%20Linux%202.6.pdf),
which are functions responsible for doing IO in swap memory. To collect the exact moment that an access to swap happens,
the collector attaches `kprobes` for cited functions.

#### Soft IRQ

The following `tracepoints` are used to measure time usage for soft IRQs:

-   [`irq/softirq_entry`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_softirq_entry): Called
     before softirq handler
-   [`irq/softirq_exit`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_softirq_exit): Called when
     softirq handler returns.

#### Hard IRQ

The following tracepoints are used to measure the latency of servicing a
hardware interrupt request (hard IRQ).

-   [`irq/irq_handler_entry`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_irq_handler_entry):
    Called immediately before the IRQ action handler.
-   [`irq/irq_handler_exit`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_irq_handler_exit):
    Called immediately after the IRQ action handler returns.
-   `irq_vectors`: These are traces from `irq_handler_entry` and
    `irq_handler_exit` when an IRQ is handled. The following elements from vector
    are triggered:
    -   `irq_vectors/local_timer_entry`
    -   `irq_vectors/local_timer_exit`
    -   `irq_vectors/reschedule_entry`
    -   `irq_vectors/reschedule_exit`
    -   `irq_vectors/call_function_entry`
    -   `irq_vectors/call_function_exit`
    -   `irq_vectors/call_function_single_entry`
    -   `irq_vectors/call_function_single_xit`
    -   `irq_vectors/irq_work_entry`
    -   `irq_vectors/irq_work_exit`
    -   `irq_vectors/error_apic_entry`
    -   `irq_vectors/error_apic_exit`
    -   `irq_vectors/thermal_apic_entry`
    -   `irq_vectors/thermal_apic_exit`
    -   `irq_vectors/threshold_apic_entry`
    -   `irq_vectors/threshold_apic_exit`
    -   `irq_vectors/deferred_error_entry`
    -   `irq_vectors/deferred_error_exit`
    -   `irq_vectors/spurious_apic_entry`
    -   `irq_vectors/spurious_apic_exit`
    -   `irq_vectors/x86_platform_ipi_entry`
    -   `irq_vectors/x86_platform_ipi_exit`

#### IPC shared memory

To monitor shared memory system call counts, Netdata attaches tracing in the following functions:

-   `shmget`: Runs when [`shmget`](https://man7.org/linux/man-pages/man2/shmget.2.html) is called.
-   `shmat`: Runs when [`shmat`](https://man7.org/linux/man-pages/man2/shmat.2.html) is called.
-   `shmdt`: Runs when [`shmdt`](https://man7.org/linux/man-pages/man2/shmat.2.html) is called.
-   `shmctl`: Runs when [`shmctl`](https://man7.org/linux/man-pages/man2/shmctl.2.html) is called.

### Memory

In the memory submenu the eBPF plugin creates two submenus **page cache** and **synchronization** with the following
organization:

-   Page Cache
    -   Page cache ratio
    -   Dirty pages
    -   Page cache hits
    -   Page cache misses
-   Synchronization
    -   File sync
    -   Memory map sync
    -   File system sync
    -   File range sync

#### Page cache hits

When the processor needs to read or write a location in main memory, it checks for a corresponding entry in the page cache.
 If the entry is there, a page cache hit has occurred and the read is from the cache.

A page cache hit is when the page cache is successfully accessed with a read operation. We do not count pages that were
added relatively recently.

#### Dirty pages

A "dirty page" is a page in the page cache that was modified after being created. Since non-dirty pages in the page cache
 have identical copies in secondary storage (e.g. hard disk drive or solid-state drive), discarding and reusing their space
 is much quicker than paging out application memory, and is often preferred over flushing the dirty pages into secondary storage
 and reusing their space.

On `cachestat_dirties` Netdata demonstrates the number of pages that were modified. This chart shows the number of calls
to the function `mark_buffer_dirty`.

#### Page cache ratio

When the processor needs to read or write in a specific memory address, it checks for a corresponding entry in the page cache.
If the processor hits a page cache (`page cache hit`), it reads the entry from the cache. If there is no entry (`page cache miss`),
 the kernel allocates a new entry and copies data from the disk. Netdata calculates the percentage of accessed files that are cached on
 memory. The ratio is calculated counting the accessed cached pages
 (without counting [dirty pages](#dirty-pages) and pages added because of read misses) divided by total access without dirty pages.

> \_\_**\_\_\_\_**<ins>Number of accessed cached pages</ins>\***\*\_\_\*\***<br/>
> Number of total accessed pages - dirty pages - missed pages

The chart `cachestat_ratio` shows how processes are accessing page cache. In a normal scenario, we expect values around
100%, which means that the majority of the work on the machine is processed in memory. To calculate the ratio, Netdata
attaches `kprobes` for kernel functions:

-   `add_to_page_cache_lru`: Page addition.
-   `mark_page_accessed`: Access to cache.
-   `account_page_dirtied`: Dirty (modified) pages.
-   `mark_buffer_dirty`: Writes to page cache.

#### Page cache misses

A page cache miss means that a page was not inside memory when the process tried to access it. This chart shows the
result of the difference for calls between functions `add_to_page_cache_lru` and `account_page_dirtied`.

#### File sync

This chart shows calls to synchronization methods, [`fsync(2)`](https://man7.org/linux/man-pages/man2/fdatasync.2.html)
and [`fdatasync(2)`](https://man7.org/linux/man-pages/man2/fdatasync.2.html), to transfer all modified page caches
for the files on disk devices. These calls block until the disk reports that the transfer has been completed. They flush
data for specific file descriptors.

#### Memory map sync

The chart shows calls to [`msync(2)`](https://man7.org/linux/man-pages/man2/msync.2.html) syscalls. This syscall flushes
changes to a file that was mapped into memory using [`mmap(2)`](https://man7.org/linux/man-pages/man2/mmap.2.html).

#### File system sync

This chart monitors calls demonstrating commits from filesystem caches to disk. Netdata attaches `tracing` for
[`sync(2)`](https://man7.org/linux/man-pages/man2/sync.2.html), and [`syncfs(2)`](https://man7.org/linux/man-pages/man2/sync.2.html).

#### File range sync

This chart shows calls to [`sync_file_range(2)`](https://man7.org/linux/man-pages/man2/sync_file_range.2.html) which
synchronizes file segments with disk.

> Note: This is the most dangerous syscall to synchronize data, according to its manual.

### Multiple Device (MD) arrays

The eBPF plugin shows multi-device flushes happening in real time. This can be used to explain some spikes happening
in [disk latency](#disk) charts.

By default, MD flush is disabled. To enable it, configure your
`/etc/netdata/ebpf.d.conf` file as:

```conf
[global]
    mdflush = yes
```

#### MD flush

To collect data related to Linux multi-device (MD) flushing, the following kprobe is used:

-   `kprobe/md_flush_request`: called whenever a request for flushing multi-device data is made.

### Disk

The eBPF plugin also shows a chart in the Disk section when the `disk` thread is enabled.

#### Disk Latency

This will create the chart `disk_latency_io` for each disk on the host. The following tracepoints are used:

-   [`block/block_rq_issue`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_block_rq_issue):
    IO request operation to a device drive.
-   [`block/block_rq_complete`](https://www.kernel.org/doc/html/latest/core-api/tracepoint.html#c.trace_block_rq_complete):
    IO operation completed by device.

Disk Latency is the single most important metric to focus on when it comes to storage performance, under most circumstances.
For hard drives, an average latency somewhere between 10 to 20 ms can be considered acceptable. For SSD (Solid State Drives),
in most cases, workloads experience less than 1 ms latency numbers, but workloads should never reach higher than 3 ms.
The dimensions refer to time intervals.

### Filesystem

This group has charts demonstrating how applications interact with the Linux kernel to open and close file descriptors.
It also brings latency charts for several different filesystems.

#### Latency Algorithm

We calculate the difference between the calling and return times, spanning disk I/O, file system operations (lock, I/O),
run queue latency and all events related to the monitored action.

#### ext4

To measure the latency of executing some actions in an
[ext4](https://elixir.bootlin.com/linux/latest/source/fs/ext4) filesystem, the
collector needs to attach `kprobes` and `kretprobes` for each of the following
functions:

-   `ext4_file_read_iter`: Function used to measure read latency.
-   `ext4_file_write_iter`: Function used to measure write latency.
-   `ext4_file_open`: Function used to measure open latency.
-   `ext4_sync_file`: Function used to measure sync latency.

#### ZFS

To measure the latency of executing some actions in a zfs filesystem, the
collector needs to attach `kprobes` and `kretprobes` for each of the following
functions:

-   `zpl_iter_read`: Function used to measure read latency.
-   `zpl_iter_write`: Function used to measure write latency.
-   `zpl_open`: Function used to measure open latency.
-   `zpl_fsync`: Function used to measure sync latency.

#### XFS

To measure the latency of executing some actions in an
[xfs](https://elixir.bootlin.com/linux/latest/source/fs/xfs) filesystem, the
collector needs to attach `kprobes` and `kretprobes` for each of the following
functions:

-   `xfs_file_read_iter`: Function used to measure read latency.
-   `xfs_file_write_iter`: Function used to measure write latency.
-   `xfs_file_open`: Function used to measure open latency.
-   `xfs_file_fsync`: Function used to measure sync latency.

#### NFS

To measure the latency of executing some actions in an
[nfs](https://elixir.bootlin.com/linux/latest/source/fs/nfs) filesystem, the
collector needs to attach `kprobes` and `kretprobes` for each of the following
functions:

-   `nfs_file_read`: Function used to measure read latency.
-   `nfs_file_write`: Function used to measure write latency.
-   `nfs_file_open`: Functions used to measure open latency.
-   `nfs4_file_open`: Functions used to measure open latency for NFS v4.
-   `nfs_getattr`: Function used to measure sync latency.

#### btrfs

To measure the latency of executing some actions in a [btrfs](https://elixir.bootlin.com/linux/latest/source/fs/btrfs/file.c)
filesystem, the collector needs to attach `kprobes` and `kretprobes` for each of the following functions:

> Note: We are listing two functions used to measure `read` latency, but we use either `btrfs_file_read_iter` or
> `generic_file_read_iter`, depending on kernel version.

-   `btrfs_file_read_iter`: Function used to measure read latency since kernel `5.10.0`.
-   `generic_file_read_iter`: Like `btrfs_file_read_iter`, but this function was used before kernel `5.10.0`.
-   `btrfs_file_write_iter`: Function used to write data.
-   `btrfs_file_open`: Function used to open files.
-   `btrfs_sync_file`: Function used to synchronize data to filesystem.

#### File descriptor

To give metrics related to `open` and `close` events, instead of attaching kprobes for each syscall used to do these
events, the collector attaches `kprobes` for the common function used for syscalls:

-   [`do_sys_open`](https://0xax.gitbooks.io/linux-insides/content/SysCall/linux-syscall-5.html): Internal function used to
     open files.
-   [`do_sys_openat2`](https://elixir.bootlin.com/linux/v5.6/source/fs/open.c#L1162):
    Function called from `do_sys_open` since version `5.6.0`.
-   [`close_fd`](https://www.mail-archive.com/linux-kernel@vger.kernel.org/msg2271761.html): Function used to close file
    descriptor since kernel `5.11.0`.
-   `__close_fd`: Function used to close files before version `5.11.0`.

#### File error

This chart shows the number of times some software tried and failed to open or close a file descriptor.

#### VFS

The Linux Virtual File System (VFS) is an abstraction layer on top of a
concrete filesystem like the ones listed in the parent section, e.g. `ext4`.

In this section we list the mechanism by which we gather VFS data, and what
charts are consequently created.

##### VFS eBPF Hooks

To measure the latency and total quantity of executing some VFS-level
functions, ebpf.plugin needs to attach kprobes and kretprobes for each of the
following functions:

-   `vfs_write`: Function used monitoring the number of successful & failed
    filesystem write calls, as well as the total number of written bytes.
-   `vfs_writev`: Same function as `vfs_write` but for vector writes (i.e. a
    single write operation using a group of buffers rather than 1).
-   `vfs_read`: Function used for monitoring the number of successful & failed
    filesystem read calls, as well as the total number of read bytes.
-   `vfs_readv` Same function as `vfs_read` but for vector reads (i.e. a single
    read operation using a group of buffers rather than 1).
-   `vfs_unlink`: Function used for monitoring the number of successful & failed
    filesystem unlink calls.
-   `vfs_fsync`: Function used for monitoring the number of successful & failed
    filesystem fsync calls.
-   `vfs_open`: Function used for monitoring the number of successful & failed
    filesystem open calls.
-   `vfs_create`: Function used for monitoring the number of successful & failed
    filesystem create calls.

##### VFS Deleted objects

This chart monitors calls to `vfs_unlink`. This function is responsible for removing objects from the file system.

##### VFS IO

This chart shows the number of calls to the functions `vfs_read` and `vfs_write`.

##### VFS IO bytes

This chart also monitors `vfs_read` and `vfs_write` but, instead of the number of calls, it shows the total amount of
bytes read and written with these functions.

The Agent displays the number of bytes written as negative because they are moving down to disk.

##### VFS IO errors

The Agent counts and shows the number of instances where a running program experiences a read or write error.

##### VFS Create

This chart shows the number of calls to `vfs_create`. This function is responsible for creating files.

##### VFS Synchronization

This chart shows the number of calls to `vfs_fsync`. This function is responsible for calling `fsync(2)` or
`fdatasync(2)` on a file. You can see more details in the Synchronization section.

##### VFS Open

This chart shows the number of calls to `vfs_open`. This function is responsible for opening files.

#### Directory Cache

Metrics for directory cache are collected using kprobe for `lookup_fast`, because we are interested in the number of
times this function is accessed. On the other hand, for `d_lookup` we are not only interested in the number of times it
is accessed, but also in possible errors, so we need to attach a `kretprobe`. For this reason, the following is used:

-   [`lookup_fast`](https://lwn.net/Articles/649115/): Called to look at data inside the directory cache.
-   [`d_lookup`](https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git/tree/fs/dcache.c?id=052b398a43a7de8c68c13e7fa05d6b3d16ce6801#n2223):
    Called when the desired file is not inside the directory cache.

##### Directory Cache Interpretation

When directory cache is showing 100% that means that every accessed file was present in the directory cache.
If files are not present in the directory cache, they are either not present in the file system or the files were not
accessed before.

### Mount Points

The following `tracing` are used to collect `mount` & `unmount` call counts:

-   [`mount`](https://man7.org/linux/man-pages/man2/mount.2.html): mount filesystem on host.
-   [`umount`](https://man7.org/linux/man-pages/man2/umount.2.html): umount filesystem on host.

### Networking Stack

Netdata monitors socket bandwidth attaching `tracing` for internal functions.

#### TCP outbound connections

This chart demonstrates calls to `tcp_v4_connection` and `tcp_v6_connection` that start connections for IPV4 and IPV6, respectively.

#### TCP inbound connections

This chart demonstrates TCP and UDP connections that the host receives.
To collect this information, netdata attaches a tracing to `inet_csk_accept`.

#### TCP bandwidth functions

This chart demonstrates calls to functions `tcp_sendmsg`, `tcp_cleanup_rbuf`, and `tcp_close`; these functions are used
to send & receive data and to close connections when `TCP` protocol is used.

#### TCP bandwidth

This chart demonstrates calls to functions:

-   `tcp_sendmsg`: Function responsible to send data for a specified destination.
-   `tcp_cleanup_rbuf`: We use this function instead of `tcp_recvmsg`, because the last one misses `tcp_read_sock` traffic
     and we would also need to add more `tracing` to get the socket and package size.
-   `tcp_close`: Function responsible to close connection.

#### TCP retransmit

This chart demonstrates calls to function `tcp_retransmit` that is responsible for executing TCP retransmission when the
receiver did not return the packet during the expected time.

#### UDP functions

This chart demonstrates calls to functions `udp_sendmsg` and `udp_recvmsg`, which are responsible for sending &
receiving data for connections when the `UDP` protocol is used.

#### UDP bandwidth

Like the previous chart, this one also monitors `udp_sendmsg` and `udp_recvmsg`, but instead of showing the number of
calls, it monitors the number of bytes sent and received.

### Apps

#### OOM Killing

These are tracepoints related to [OOM](https://en.wikipedia.org/wiki/Out_of_memory) killing processes.

-   `oom/mark_victim`: Monitors when an oomkill event happens.

## Known issues

### Performance opimization

eBPF monitoring is complex and produces a large volume of metrics. We've discovered scenarios where the eBPF plugin
significantly increases kernel memory usage by several hundred MB.

When the integration with apps or cgroup is enabled, the eBPF collector allocates memory for each process running. If your
node is experiencing high memory usage and there is no obvious culprit to be found in the `apps.mem` chart, consider:

- Modify [maps per core](#maps-per-core) to use only one map.
- Disable [integration with apps](#integration-with-appsplugin).
- Disable [integration with cgroup](#integration-with-cgroupsplugin).

If with these changes you still suspect eBPF using too much memory, and there is no obvious culprit to be found 
in the `apps.mem` chart, consider testing for high kernel memory usage by [disabling eBPF monitoring](#configuring-ebpfplugin).
Next, [restart Netdata](https://github.com/netdata/netdata/blob/master/packaging/installer/README.md#maintaining-a-netdata-agent-installation) with
`sudo systemctl restart netdata` to see if system memory usage (see the `system.ram` chart) has dropped significantly.

Beginning with `v1.31`, kernel memory usage is configurable via the [`pid table size` setting](#pid-table-size)
in `ebpf.conf`.

The total memory usage is a well known [issue](https://lore.kernel.org/all/167821082315.1693.6957546778534183486.git-patchwork-notify@kernel.org/) 
for eBPF, this is not a bug present in plugin.

### SELinux

When [SELinux](https://www.redhat.com/en/topics/linux/what-is-selinux) is enabled, it may prevent `ebpf.plugin` from
starting correctly. Check the Agent's `error.log` file for errors like the ones below:

```bash
2020-06-14 15:32:08: ebpf.plugin ERROR : EBPF PROCESS : Cannot load program: /usr/libexec/netdata/plugins.d/pnetdata_ebpf_process.3.10.0.o (errno 13, Permission denied)
2020-06-14 15:32:19: netdata ERROR : PLUGINSD[ebpf] : read failed: end of file (errno 9, Bad file descriptor)
```

You can also check for errors related to `ebpf.plugin` inside `/var/log/audit/audit.log`:

```bash
type=AVC msg=audit(1586260134.952:97): avc:  denied  { map_create } for  pid=1387 comm="ebpf.pl" scontext=system_u:system_r:unconfined_service_t:s0 tcontext=system_u:system_r:unconfined_service_t:s0 tclass=bpf permissive=0
type=SYSCALL msg=audit(1586260134.952:97): arch=c000003e syscall=321 success=no exit=-13 a0=0 a1=7ffe6b36f000 a2=70 a3=0 items=0 ppid=1135 pid=1387 auid=4294967295 uid=994 gid=990 euid=0 suid=0 fsuid=0 egid=990 sgid=990 fsgid=990 tty=(none) ses=4294967295 comm="ebpf_proc
ess.pl" exe="/usr/libexec/netdata/plugins.d/ebpf.plugin" subj=system_u:system_r:unconfined_service_t:s0 key=(null)
```

If you see similar errors, you will have to adjust SELinux's policies to enable the eBPF collector.

#### Creation of bpf policies

To enable `ebpf.plugin` to run on a distribution with SELinux enabled, it will be necessary to take the following
actions.

First, stop the Netdata Agent.

```bash
# systemctl stop netdata
```

Next, create a policy with the `audit.log` file you examined earlier.

```bash
# grep ebpf.plugin /var/log/audit/audit.log | audit2allow -M netdata_ebpf
```

This will create two new files: `netdata_ebpf.te` and `netdata_ebpf.mod`.

Edit the `netdata_ebpf.te` file to change the options `class` and `allow`. You should have the following at the end of
the `netdata_ebpf.te` file.

```conf
module netdata_ebpf 1.0;
require {
        type unconfined_service_t;
        class bpf { map_create map_read map_write prog_load prog_run };
}
#============= unconfined_service_t ==============
allow unconfined_service_t self:bpf { map_create map_read map_write prog_load prog_run };
```

Then compile your `netdata_ebpf.te` file with the following commands to create a binary that loads the new policies:

```bash
# checkmodule -M -m -o netdata_ebpf.mod netdata_ebpf.te
# semodule_package -o netdata_ebpf.pp -m netdata_ebpf.mod
```

Finally, you can load the new policy and start the Netdata agent again:

```bash
# semodule -i netdata_ebpf.pp
# systemctl start netdata
```

### Linux kernel lockdown

Beginning with [version 5.4](https://www.zdnet.com/article/linux-to-get-kernel-lockdown-feature/), the Linux kernel has
a feature called "lockdown," which may affect `ebpf.plugin` depending how the kernel was compiled. The following table
shows how the lockdown module impacts `ebpf.plugin` based on the selected options:

| Enforcing kernel lockdown | Enable lockdown LSM early in init | Default lockdown mode | Can `ebpf.plugin` run with this? |
| :------------------------ | :-------------------------------- | :-------------------- | :------------------------------- |
| YES                       | NO                                | NO                    | YES                              |
| YES                       | Yes                               | None                  | YES                              |
| YES                       | Yes                               | Integrity             | YES                              |
| YES                       | Yes                               | Confidentiality       | NO                               |

If you or your distribution compiled the kernel with the last combination, your system cannot load shared libraries
required to run `ebpf.plugin`.

## Functions

### ebpf_thread

The eBPF plugin has a [function](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) named
`ebpf_thread` that controls its internal threads and helps to reduce the overhead on host. Using the function you
can run the plugin with all threads disabled and enable them only when you want to take a look in specific areas.

#### List threads

To list all threads status you can query directly the endpoint function:

`http://localhost:19999/api/v1/function?function=ebpf_thread`

It is also possible to query a specific thread adding keyword `thread` and thread name:

`http://localhost:19999/api/v1/function?function=ebpf_thread%20thread:mount`

#### Enable thread

It is possible to enable a specific thread using the keyword `enable`:

`http://localhost:19999/api/v1/function?function=ebpf_thread%20enable:mount`

this will run thread `mount` during 300 seconds (5 minutes). You can specify a specific period by appending the period
after the thread name:

`http://localhost:19999/api/v1/function?function=ebpf_thread%20enable:mount:600`

in this example thread `mount` will run during 600 seconds (10 minutes).

#### Disable thread

It is also possible to stop any thread running using the keyword `disable`. For example, to disable `cachestat` you can
request:

`http://localhost:19999/api/v1/function?function=ebpf_thread%20disable:cachestat`

#### Debugging threads

You can verify the impact of threads on the host by running the
[ebpf_thread_function.sh](https://github.com/netdata/netdata/blob/master/tests/ebpf/ebpf_thread_function.sh)
script on your environment.

You can check the results of having threads running on your environment in the Netdata monitoring section on your
dashboard

<img src="https://github.com/netdata/netdata/assets/49162938/91823573-114c-4c16-b634-cc46f7bb1bcf" alt="Threads running." />

### ebpf_socket

The eBPF plugin has a [function](https://github.com/netdata/netdata/blob/master/docs/cloud/netdata-functions.md) named
`ebpf_socket` that shows the current status of open sockets on host.

#### Families

The plugin shows by default sockets for IPV4 and IPV6, but it is possible to select a specific family by passing the
family as an argument:

`http://localhost:19999/api/v1/function?function=ebpf_socket%20family:IPV4`

#### Resolve

The plugin resolves ports to service names by default. You can show the port number by disabling the name resolution:

`http://localhost:19999/api/v1/function?function=ebpf_socket%20resolve:NO`

#### CIDR

The plugin shows connections for all possible destination IPs by default. You can limit the range by specifying the CIDR:

`http://localhost:19999/api/v1/function?function=ebpf_socket%20cidr:192.168.1.0/24`

#### PORT

The plugin shows connections for all possible ports by default. You can limit the range by specifying a port or range
of ports:

`http://localhost:19999/api/v1/function?function=ebpf_socket%20port:1-1024`
