<!--
title: "eBPF monitoring with Netdata"
description: "Use Netdata's extended Berkeley Packet Filter (eBPF) collector to monitor kernel-level metrics about your complex applications with per-second granularity."
custom_edit_url: https://github.com/netdata/netdata/edit/master/collectors/ebpf.plugin/README.md
sidebar_label: "eBPF"
-->

# eBPF monitoring with Netdata

Netdata's extended Berkeley Packet Filter (eBPF) collector monitors kernel-level metrics for file descriptors, virtual
filesystem IO, and process management on Linux systems. You can use our eBPF collector to analyze how and when a process
accesses files, when it makes system calls, whether it leaks memory or creating zombie processes, and more.

Netdata's eBPF monitoring toolkit uses two custom eBPF programs. The default, called `entry`, monitors calls to a
variety of kernel functions, such as `do_sys_open`, `__close_fd`, `vfs_read`, `vfs_write`, `_do_fork`, and more. The
`return` program also monitors the return of each kernel functions to deliver more granular metrics about how your
system and its applications interact with the Linux kernel.

eBPF monitoring can help you troubleshoot and debug how applications interact with the Linux kernel. See our [guide on
troubleshooting apps with eBPF metrics](/docs/guides/troubleshoot/monitor-debug-applications-ebpf.md) for configuration
and troubleshooting tips.

<figure>
  <img src="https://user-images.githubusercontent.com/1153921/74746434-ad6a1e00-5222-11ea-858a-a7882617ae02.png" alt="An example of VFS charts, made possible by the eBPF collector plugin" />
  <figcaption>An example of VFS charts made possible by the eBPF collector plugin.</figcaption>
</figure>

## Enable the collector on Linux

**The eBPF collector is installed and enabled by default on most new installations of the Agent**. The eBPF collector
does not currently work with [static build installations](/packaging/installer/methods/kickstart-64.md), but improved
support is in active development.

eBPF monitoring only works on Linux systems and with specific Linux kernels, including all kernels newer than `4.11.0`,
and all kernels on CentOS 7.6 or later.

If your Agent is v1.22 or older, you may to enable the collector yourself. See the [configuration](#configuration)
section for details.

## Charts

The eBPF collector creates an **eBPF** menu in the Agent's dashboard along with three sub-menus: **File**, **VFS**, and
**Process**. All the charts in this section update every second. The collector stores the actual value inside of its
process, but charts only show the difference between the values collected in the previous and current seconds.

### File

This group has two charts demonstrating how software interacts with the Linux kernel to open and close file descriptors.

#### File descriptor

This chart contains two dimensions that show the number of calls to the functions `do_sys_open` and `__close_fd`. Most
software do not commonly call these functions directly, but they are behind the system calls `open(2)`, `openat(2)`,
and `close(2)`.

#### File error

This chart shows the number of times some software tried and failed to open or close a file descriptor.

### VFS

A [virtual file system](https://en.wikipedia.org/wiki/Virtual_file_system) (VFS) is a layer on top of regular
filesystems. The functions present inside this API are used for all filesystems, so it's possible the charts in this
group won't show _all_ the actions that occurred on your system.

#### Deleted objects

This chart monitors calls for `vfs_unlink`. This function is responsible for removing objects from the file system.

#### IO

This chart shows the number of calls to the functions `vfs_read` and `vfs_write`.

#### IO bytes

This chart also monitors `vfs_read` and `vfs_write`, but instead shows the total of bytes read and written with these
functions.

The Agent displays the number of bytes written as negative because they are moving down to disk.

#### IO errors

The Agent counts and shows the number of instances where a running program experiences a read or write error.

### Process

For this group, the eBPF collector monitors process/thread creation and process end, and then displays any errors in the
following charts.

#### Process thread

Internally, the Linux kernel treats both processes and threads as `tasks`. To create a thread, the kernel offers a few
system calls: `fork(2)`, `vfork(2)` and `clone(2)`. In turn, each of these system calls use the function `_do_fork`. To
generate this chart, the eBPF collector monitors `_do_fork` to populate the `process` dimension, and monitors
`sys_clone` to identify threads.

#### Exit

Ending a task requires two steps. The first is a call to the internal function `do_exit`, which notifies the operating
system that the task is finishing its work. The second step is to release the kernel information with the internal
function `release_task`. The difference between the two dimensions can help you discover [zombie
processes](https://en.wikipedia.org/wiki/Zombie_process).

#### Task error

The functions responsible for ending tasks do not return values, so this chart contains information about failures on
process and thread creation.

## Configuration

Enable or disable the entire eBPF collector by editing `netdata.conf`.

```bash
cd /etc/netdata/   # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config netdata.conf
```

To enable the collector, scroll down to the `[plugins]` section ensure the relevant line references `ebpf` (not
`ebpf_process`), is uncommented, and is set to `yes`.

```conf
[plugins]
   ebpf = yes
```

You can also configure the eBPF collector's behavior by editing `ebpf.conf`.

```bash
cd /etc/netdata/   # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config ebpf.conf
```

### `[global]`

The `[global]` section defines settings for the whole eBPF collector.

#### ebpf load mode

The collector has two different eBPF programs. These programs monitor the same functions inside the kernel, but they
monitor, process, and display different kinds of information.

By default, this plugin uses the `entry` mode. Changing this mode can create significant overhead on your operating
system, but also offer valuable information if you are developing or debugging software. The `ebpf load mode` option
accepts the following values: ​

-   `entry`: This is the default mode. In this mode, the eBPF collector only monitors calls for the functions described
    in the sections above, and does not show charts related to errors.
-   `return`: In the `return` mode, the eBPF collector monitors the same kernel functions as `entry`, but also creates
    new charts for the return of these functions, such as errors. Monitoring function returns can help in debugging
    software, such as failing to close file descriptors or creating zombie processes.
    
#### Integration with `apps.plugin`

The eBPF collector also creates charts for each running application through an integration with the
[`apps.plugin`](/collectors/apps.plugin/README.md). This integration helps you understand how specific applications
interact with the Linux kernel.

When the integration is enabled, your dashboard will also show the following charts using low-level Linux metrics:
    
-   eBPF syscall    
    -   Number of calls to open files.
    -   Number of files closed.
    -   Number of calls to delete files.
    -   Number of calls to `vfs_write`.
    -   Number of calls to `vfs_read`.
    -   Number of bytes written trough `vfs_write`
    -   Number of bytes read trough `vfs_read`
    -   Number of process created trough `do_fork`
    -   Number of threads created trough `do_fork` or `__x86_64_sys_clone`, depending on your system's kernel version.
    -   Number of times that a process called `do_exit`. 
    -   Number of calls to open files that returned errors.
    -   Number of calls to close files that returned errors.
    -   Number of calls to read a file that returned errors.
    -   Number of calls to read a file that returned errors.
-   eBPF net
    -   Number of bytes transmited per seconds.   

If you want to _disable_ the integration with `apps.plugin` along with the above charts, change the setting `disable
apps` to `yes`.

```conf
[global]
   disable apps = yes
```

### `[ebpf programs]`

The eBPF collector enables and runs the following eBPF programs by default:

-   `process`: This eBPF program creates charts that show information about process creation, VFS IO, and files removed.
    When in `return` mode, it also creates charts showing errors when these operations are executed.
-   `network viewer`: This eBPF program creates charts with information about `TCP` and `UDP` functions, including the
    bandwidth consumed by each.

### `[network viewer]`

You can configure the information shown on `outbound` and `inbound` charts with the settings in this section. 

```conf
[network viewer]
    maximum dimensions = 500
    resolve hostname ips = no
    ports = 1-1024 !145 !domain
    hostnames = !example.com
    ips = !127.0.0.1/8 10.0.0.0/8 172.16.0.0/12 192.168.0.0/16 fc00::/7
```

When you define a `ports` setting, Netdata will collect network metrics for that specific port. For example, if you
write `ports = 19999`, Netdata will collect only connections for itself. The `hostnames` setting accepts  
[simple patterns](/libnetdata/simple_pattern/README.md).  The `ports`, and `ips` settings accept negation (`!`) to
 deny specific values or asterisk alone to define all values.

In the above example, Netdata will collect metrics for all ports between 1 and 443, with the exception of 53 (domain)
and 145.

The following options are available:

-   `ports`: Define the destination ports for Netdata to monitor.
-   `hostnames`: The list of hostnames that can be resolved to an IP address. 
-   `ips`: The IP or range of IPs that you want to monitor. You can use IPv4 or IPv6 addresses, use dashes to define a
    range of IPs, or use CIDR values. The default behavior is to only collect data for private IP addresess, but this
    can be changed with the `ips` setting.
    
By default, Netdata displays up to 500 dimensions on network viewer charts. If there are more possible dimensions, they
will be bundled into the `other` dimension. You can increase the number of shown dimensions by changing the `maximum
dimensions` setting.

The dimensions for the traffic charts are created using the destination IPs of the sockets by default. This can be
changed setting `resolve hostname ips = yes` and restarting Netdata, after this Netdata will create dimensions using
the `hostnames` every time that is possible to resolve IPs to their hostnames.

### `[service name]`

Netdata uses the list of services in `/etc/services` to plot network viewer charts. If this file does not contain the
name for a particular service you use in your infrastructure, you will need to add it to the `[service name]` section.

For example, Netdata's default port (`19999`) is not listed in `/etc/services`. To associate that port with the Netdata
service in network viewer charts, and thus see the name of the service instead of its port, define it:

```conf
[service name]
    19999 = Netdata
```

## Troubleshooting

If the eBPF collector does not work, you can troubleshoot it by running the `ebpf.plugin` command and investigating its
output.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo -u netdata bash
./ebpf.plugin
```

You can also use `grep` to search the Agent's `error.log` for messages related to eBPF monitoring.

```bash
grep -i ebpf /var/log/netdata/error.log
```

### Confirm kernel compatibility

The eBPF collector only works on Linux systems and with specific Linux kernels. We support all kernels more recent than
`4.11.0`, and all kernels on CentOS 7.6 or later.

You can run our helper script to determine whether your system can support eBPF monitoring.

```bash
curl -sSL https://raw.githubusercontent.com/netdata/kernel-collector/master/tools/check-kernel-config.sh | sudo bash
```

If this script returns no output, your system is ready to compile and run the eBPF collector.

If you see a warning about a missing kerkel configuration (`KPROBES KPROBES_ON_FTRACE HAVE_KPROBES BPF BPF_SYSCALL
BPF_JIT`), you will need to recompile your kernel to support this configuration. The process of recompiling Linux
kernels varies based on your distribution and version. Read the documentation for your system's distribution to learn
more about the specific workflow for recompiling the kernel, ensuring that you set all the necessary 

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
mount these filesystems on startup. More information can be found in the [ftrace documentation](https://www.kernel.org/doc/Documentation/trace/ftrace.txt).

## Performance

Because eBPF monitoring is complex, we are evaluating the performance of this new collector in various real-world
conditions, across various system loads, and when monitoring complex applications.

Our [initial testing](https://github.com/netdata/netdata/issues/8195) shows the performance of the eBPF collector is
nearly identical to our [apps.plugin collector](/collectors/apps.plugin/README.md), despite collecting and displaying
much more sophisticated metrics. You can now use the eBPF to gather deeper insights without affecting the performance of
your complex applications at any load.

## SELinux

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

### Creation of bpf policies

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

## Lockdown

Beginning with [version 5.4](https://www.zdnet.com/article/linux-to-get-kernel-lockdown-feature/), the Linux kernel has
a feature called "lockdown," which may affect `ebpf.plugin` depending how the kernel was compiled. The following table
shows how the lockdown module impacts `ebpf.plugin` based on the selected options:

| Enforcing kernel lockdown | Enable lockdown LSM early in init | Default lockdown mode | Can `ebpf.plugin` run with this? |
|:------------------------- |:--------------------------------- |:--------------------- |:-------------------------------- |
| YES                       | NO                                | NO                    | YES                              |
| YES                       | Yes                               | None                  | YES                              |
| YES                       | Yes                               | Integrity             | YES                              |
| YES                       | Yes                               | Confidentiality       | NO                               |

If you or your distribution compiled the kernel with the last combination, your system cannot load shared libraries
required to run `ebpf.plugin`.

## Cleaning `kprobe_events`
The eBPF collector adds entries to the file `/sys/kernel/debug/tracing/kprobe_events`, and cleans them on exit, unless
another process prevents it. If you need to clean the eBPF entries safely, you can manually run the script
`/usr/libexec/netdata/plugins.d/reset_netdata_trace.sh`.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Febpf.plugin%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
