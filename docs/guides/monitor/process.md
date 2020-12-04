<!--
title: Monitor any process in real-time with Netdata
description: TK
image: /img/seo/guides/monitor/process.png
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/guides/monitor/process.md
-->

# Monitor any process in real-time with Netdata

TK

## Prerequisites

-   One or more Linux nodes running the [Netdata Agent](/docs/get/README.md).
-   A general understanding of how to [configure the Netdata Agent](/docs/configure/nodes.md) using `edit-config`.
-   A Netdata Cloud account. [Sign up](https://app.netdata.cloud) if you don't have one already.

## How does Netdata do process monitoring?

The Netdata Agent already knows to look for hundreds of [standard applications that we support via
collectors](/collectors/COLLECTORS.md), and groups them based on their purpose. Let's say you want to monitor a MySQL
database using its process. The Netdata Agent already knows to look for processes with the string `mysqld` in their
name, along with a few others, and puts them into the `sql` group. This `sql` group then becomes a dimension in all
process-specific charts.

The process and groups settings are used by two unique and powerful collectors.

[**apps.plugin**](/collectors/apps.plugin/README.md) looks at the Linux process tree every second, much like `top` or
`ps fax`, and collects resource utilization information on every running process. It then automatically adds a layer of
meaningful visualization on top of these metrics, and creates per-application charts under the **Applications** section
of a Netdata dashboard.

[**ebpf.plugin**](/collectors/ebpf.plugin/README.md): Netdata's extended Berkeley Packet Filter (eBPF) collector
monitors Linux kernel-level metrics for file descriptors, virtual filesystem IO, and process management, and then hands
process-specific metrics over to `apps.plugin` for visualization. The eBPF collector also collects and visualizes
metrics on an _event frequency_, which is even more precise than Netdata's standard per-second granularity. You can find
these metrics in the **ebpf syscall** and **ebpf net** sub-sections under **Applications**.

With these collectors working in parallel, you can visualize the following per-second metrics for _any_ process on your
Linux systems:

-   CPU utilization (`apps.cpu`)
    -   Total CPU usage
    -   User/system CPU usage (`apps.cpu_user`/`apps.cpu_system`)
-   Disk I/O
    -   Physical reads/writes (`apps.preads`/`apps.pwrites`)
    -   Logical reads/writes (`apps.lreads`/`apps.lwrites`)
    -   Open unique files (if a file is found open multiple times, it is counted just once, `apps.files`)
-   Memory
    -   Real Memory Used (non-shared, `apps.mem`)
    -   Virtual Memory Allocated (`apps.vmem`)
    -   Minor page faults (i.e. memory activity, `apps.minor_faults`)
-   Processes
    -   Threads running (`apps.threads`)
    -   Processes running (`apps.processes`)
    -   Carried over uptime (since the last Netdata Agent restart, `apps.uptime`)
    -   Minimum uptime (`apps.uptime_min`)
    -   Average uptime (`apps.uptime_average`)
    -   Maximum uptime (`apps.uptime_max`)
    -   Pipes open (`apps.pipes`)
-   Swap memory
    -   Swap memory used (`apps.swap`)
    -   Major page faults (i.e. swap activiy, `apps.major_faults`)
-   Network
    -   Sockets open (`apps.sockets`)
-   eBPF syscall
    -   Number of calls to open files. (`apps.file_open`)
    -   Number of files closed. (`apps.file_closed`)
    -   Number of calls to delete files. (`apps.file_deleted`)
    -   Number of calls to `vfs_write`. (`apps.vfs_write_call`)
    -   Number of calls to `vfs_read`. (`apps.vfs_read_call`)
    -   Number of bytes written with `vfs_write`. (`apps.vfs_write_bytes`)
    -   Number of bytes read with `vfs_read`. (`apps.vfs_read_bytes`)
    -   Number of process created with `do_fork`. (`apps.process_create`)
    -   Number of threads created with `do_fork` or `__x86_64_sys_clone`, depending on your system's kernel version. (`apps.thread_create`)
    -   Number of times that a process called `do_exit`. (`apps.task_close`)
-   eBPF net
    -   Number of bytes sent per seconds. (`apps.bandwidth_sent`)
    -   Number of bytes received. (`apps.bandwidth_recv`)

As an example, here's the per-process CPU utilization chart, including a `sql` group/dimension.

![A per-process CPU utilization chart in Netdata
Cloud](https://user-images.githubusercontent.com/1153921/101217226-3a5d5700-363e-11eb-8610-aa1640aefb5d.png)

## Configure the Netdata Agent to recognize a specific process

To monitor any process, you need to make sure the Netdata Agent is aware of it. As mentioned above, the Agent is already
aware of hundreds of processes, and collects metrics from them automatically.

But, if you want to change the grouping behavior, add an application that isn't yet supported in the Netdata Agent, or
monitor a custom application, you need to edit the `apps_groups.conf` configuration file.

Navigate to your [Netdata config directory](/docs/configure/nodes.md) and use `edit-config` to edit the file.

```bash
cd /etc/netdata   # Replace this with your Netdata config directory if not at /etc/netdata.
sudo ./edit-config apps_groups.conf
```

See the following two sections for details based on your needs. If you don't need to configure `apps_groups.conf`, jump
down to [visualizing process metrics](#visualize-process-metrics).

### Standard applications (web servers, databases, containers, and more)



If you're not running any other types of SQL databases on that node, you don't need to change the grouping, since you
know that any MySQL is the only process contributing to the `sql` group. But, if you are running multiple database
types, you can separate them into more specific groups.

### Custom applications

To start troubleshooting an application with eBPF metrics, you need to ensure your Netdata dashboard collects and
displays those metrics independent from any other process.

You can use the `apps_groups.conf` file to configure which applications appear in charts generated by
[`apps.plugin`](/collectors/apps.plugin/README.md). Once you edit this file and create a new group for the application
you want to monitor, you can see how it's interacting with the Linux kernel via real-time eBPF metrics.

Let's assume you have an application that runs on the process `custom-app`. To monitor eBPF metrics for that application
separate from any others, you need to create a new group in `apps_groups.conf` and associate that process name with it.

Open the `apps_groups.conf` file in your Netdata config directory.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory
sudo ./edit-config apps_groups.conf
```

Scroll down past the explanatory comments and stop when you see `# NETDATA processes accounting`. Above that, paste in
the following text, which creates a new `custom-app` group with the `custom-app` process. Replace `custom-app` with the
name of your application's process. `apps_groups.conf` should now look like this:

```conf
...
# -----------------------------------------------------------------------------
# Custom applications to monitor with apps.plugin and ebpf.plugin

custom-app: custom-app

# -----------------------------------------------------------------------------
# NETDATA processes accounting
...
```

## Visualize process metrics

### Using Netdata's application collector

### Using Netdata's eBPF collector

## What's next?

TK

If you want more specific metrics from your custom application, check out Netdata's [statd
support](/collectors/statsd.plugin/README.md). With statd, you can send detailed metrics from your application to
Netdata and visualize them with per-second granularity.

### Related reference documentation

-   [Netdata Agent · `apps.plugin`](/collectors/apps.plugin/README.md)
-   [Netdata Agent · `ebpf.plugin`](/collectors/ebpf.plugin/README.md)

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Fguides%2Fmonitor%2Fprocess&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
