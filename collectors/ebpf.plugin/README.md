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

We expect eBPF monitoring to be particularly valuable in observing and debugging how the Linux kernel handles custom
applications.

<figure>
  <img src="https://user-images.githubusercontent.com/1153921/74746434-ad6a1e00-5222-11ea-858a-a7882617ae02.png" alt="An example of VFS charts, made possible by the eBPF collector plugin" />
  <figcaption>An example of VFS charts made possible by the eBPF collector plugin.</figcaption>
</figure>

## Enable the collector on Linux

**The eBPF collector is installed and enabled by default on new nightly installations of the Agent**. eBPF monitoring
only works on Linux systems and with specific Linux kernels, including all kernels newer than `4.11.0`, and all kernels
on CentOS 7.6 or later._

If your Agent is v1.22 or older, you will need to explicitly [install the eBPF collector and enable
it](#install-and-enable-the-eBPF-collector).

## Charts

The eBPF collector creates an **eBPF** menu in the Agent's dashboard along with three sub-menus: **File**, **VFS**, and
**Process**. All the charts in this section update every second. The collector stores the actual value inside of its
process, but charts only show the difference between the values collected in the previous and current seconds.

### File

This group has two charts to demonstrate how software interacts with the Linux kernel to open and close file
descriptors.

#### File descriptor

This chart contains two dimensions that show the number of calls to the functions `do_sys_open` and `__close_fd`. These
functions are not commonly called from software, but they are behind the system cals `open(2)`, `openat(2)`, and
`close(2)`.

#### File error

This charts demonstrate the number of times some software tried and failed to open or close a file descriptor.

### VFS

A [virtual file system](https://en.wikipedia.org/wiki/Virtual_file_system) (VFS) is a layer on top of regular
filesystems. The functions present inside this API are used for all filesystems, so it's possible the charts in this
group won't show _all_ the actions that occured on your system.

#### Deleted objects

This chart monitors calls for `vfs_unlink`. This function is responsible for removing object from the file system.

#### IO

This chart shows the number of calls to the functions `vfs_read` and `vfs_write`.

#### IO bytes

This chart also monitors `vfs_read` and `vfs_write`, but instead shows the total of bytes read and written with these
functions.

Netdata displays the number of bytes written as negative, because they are moving down to disk.

#### IO errors

Netdata counts and shows the number of instances where a running program experiences a read or write error.

### Process

For this group, the eBPF collector monitors process/thread creation and process end, and then displays any errors in the
following charts.

#### Process thread

Internally, the Linux kernel treats both process and threads as `tasks`. To create a thread, the kernel offers a few
system calls: `fork(2)`, `vfork(2)` and `clone(2)`. Each of these system calls in turn use the function `_do_fork`. To
generate this chart, Netdata monitors `_do_fork` to populate the `process` dimension, and monitors `sys_clone` to
identify threads

#### Exit

Ending a task is actually two steps. The first is a call to the internal function `do_exit`, which notifies the
operating system that the task is finishing its work. The second step is the release of kernel information, which is
done with the internal function `release_task`. The difference between the two dimensions can help you discover [zombie
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

You can also configure the eBPF collector's behavior by editing `ebpf.conf`.

```bash
cd /etc/netdata/   # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config ebpf.conf
```

### `[global]`

In this section we define variables applied to the whole collector and the other subsections.

#### load

The collector has two different eBPF programs. These programs monitor the same functions inside the kernel, but they
monitor, process, and display different kinds of information.

By default, this plugin uses the `entry` mode. Changing this mode can create significant overhead on your operating
system, but also offer important information if you are developing or debugging software. The `load` option accepts the
following values: ​

-   `entry`: This is the default mode. In this mode, the eBPF collector only monitors calls for the functions described
    in the sections above, and does not show charts related to errors.
-   `return`: In the `return` mode, the eBPF collector monitors the same kernel functions as `entry`, but also creates
    new charts for the return of these functions, such as errors. Monitoring function returns can help in debugging
    software, such as failing to close file descriptors or creating zombie processes.

## Install and enable the eBPF collector

Systems running _v1.22 or earlier_ of the Agent will need to both install and enable the eBPF collector manually. The
collecetor is enabled by default on later versions of the Agent.

If you installed via the one-line installation script, 64-bit binary, or manually, you can append the `--enable-ebpf`
option when you reinstall.

For example, if you used the [one-line installation
script](/packaging/installer/README.md#automatic-one-line-installation-script), you can reinstall Netdata with the
following:

```bash
bash <(curl -Ss https://my-netdata.io/kickstart.sh) --enable-ebpf
```

Next, enable the collector in `netdata.conf`. Use `edit-config` to open `netdata.conf`.

```bash
cd /etc/netdata/   # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config netdata.conf
```

Scroll down to the `[plugins]` section and uncomment the `ebpf` line after changing its setting to `yes`.

```conf
[plugins]
   ebpf = yes
```

Restart Netdata with `service netdata restart`, or the appropriate method for your system, and reload your browser to
see eBPF charts.

## Troubleshooting

If the eBPF collector does not work, you can troubleshoot it by running the `ebpf.plugin` command and investigating its output.

```bash
cd /usr/libexec/netdata/plugins.d/
sudo -u netdata bash
./ebpf.plugin
```

You can also use `grep` to search the Agent's `error.log` for messages related to eBPF monitoring.

```bash
grep ebpf /var/log/netdata/error.log
```

### Confirm kernel compatibility

The eBPF collector only works on Linux systems and with specific Linux kernels. We support all kernels more recent than
`4.11.0`, and all kernels on CentOS 7.6 or later.

In addition, the kernel must be compiled with the option `CONFIG_KPROBES=y`. You can verify whether your kernel has this
option enabled by running the following commands:

```bash
grep CONFIG_KPROBES=y /boot/config-$(uname -r)
zgrep CONFIG_KPROBES=y /proc/config.gz
```

If `Kprobes` is enabled, you will see `CONFIG_KPROBES=y` as the command's output, and can skip ahead to the next step:
[mount `debugfs` and `tracefs`](#mount-debugfs-and-tracefs).

If you don't see `CONFIG_KPROBES=y` for any of the commands above, you will have to recompile your kernel to enable it.

The process of recompiling Linux kernels varies based on your distribution and version. Read the documentation for your
system's distribution to learn more about the specific workflow for recompiling the kernel, ensuring that you set the
`CONFIG_KPROBES` setting to `y` in the process.

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

If they are already mounted, you will see an error. If they are not mounted, they should be after running those two
commands. You can also configure your system's `/etc/fstab` configuration to mount these filesystems.

## Performance

Because eBPF monitoring is complex, we are evaluating the performance of this new collector in various real-world
conditions, across various system loads, and when monitoring complex applications.

Our [initial testing](https://github.com/netdata/netdata/issues/8195) shows the performance of the eBPF collector is
nearly identical to our [apps.plugin collector](/collectors/apps.plugin/README.md), despite collecting and displaying
much more sophisticated metrics. You can now use the eBPF to gather deeper insights without affecting the performance of
your complex applications at any load.
