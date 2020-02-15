# ebpf_process.plugin

This plugin uses eBPF to monitor system calls inside your operating system's kernel. For now, the main goal of this
plugin is to monitor IO and process management on the host where it is running. 

This plugin has different configuration modes, all of which can be adjusted with its configuration file at
`ebpf_process.conf`. By default, the plugin uses the less expensive `entry` mode. You can learn more about how the
plugin works using `entry` by reading this configuration file.

You can always edit this file with `edit-config`:

```bash
cd /etc/netdata/ # Replace with your Netdata configuration directory, if not /etc/netdata/
./edit-config ebpf_process.conf
```

## Enable the plugin on Linux

Currently, `ebpf_process` only works on Linux systems. 

To enable this plugin and its collector, your operating system's kernel must be more recent than `4.11.0`, and it must
be compiled with the option `CONFIG_KPROBES=y`. You can verify whether your kernel has this option enabled by running
the following commands:

```bash
# grep CONFIG_KPROBES=y /boot/config-$(uname -r)
# zgrep CONFIG_KPROBES=y /proc/config.gz
```

If `Kprobes` is enabled, you will see `CONFIG_KPROBES=y` as the command's output. If you don't see `CONFIG_KPROBES=y`
for any of the commands above, you will have to recompile your kernel to enable it. See the next step, [Recompiling your
kernel](#recompile-your-kernel), for details.

You also need to have both the `tracefs` and `debugfs` filesystems mounted on your system.

### Recompile your kernel

The process of recompiling Linux kernels varies based on your distribution and version. Read the documentation for your
system's distribution to learn more about the specific workflow for 

-   [Ubuntu](https://wiki.ubuntu.com/Kernel/BuildYourOwnKernel)
-   [Debian](https://kernel-team.pages.debian.net/kernel-handbook/ch-common-tasks.html#s-common-official)
-   [Fedora](https://fedoraproject.org/wiki/Building_a_custom_kernel)
-   [CentOS](https://wiki.centos.org/HowTos/Custom_Kernel)
-   [Arch Linux](https://wiki.archlinux.org/index.php/Kernel/Traditional_compilation)
-   [Slackware](https://docs.slackware.com/howtos:slackware_admin:kernelbuilding)

### Mount `debugfs` and `tracefs`

Try mounting the `tracefs` and `debugfs` filesystems using the commands below:

```bash
# mount -t debugfs nodev /sys/kernel/debug
# mount -t tracefs nodev /sys/kernel/tracing
```
​
If they are already mounted, you will see an error. You can also configure your system's `/etc/fstab` configuration to 
mount these filesystems.

## Enable the eBPF plugin
The plugin is disabled by default because it adds overhead to the system running the Netdata agent.

To enable it, use `edit-config` to open `netdata.conf` and set `ebpf_process = yes` in the `[plugins]` section.

```conf
[plugins]
   ebpf_process = yes
```

## Charts

The first version of `ebpf_process.plugin` gives a general vision about process running on computer. The charts related
to this plugin are inside the **eBPF** option on dashboard menu and divided in three groups `file`, `vfs`, and
`process`.

All the collector charts show values per second. The collector retains the total value, but charts only show the
difference between the previous and current metrics collections.

### File

This group has two charts to demonstrate how software interacts with the Linux kernel to open and close file 
descriptors.

#### File descriptor

This chart contain two dimensions that show the number of calls to the functions `do_sys_open` and `__close_fd`. These
functions are not commonly called from software, but they are behind the system cals `open(2)`, `openat(2)`, and
`close(2)`. ​

#### File error

This charts demonstrate the number of times some software tried and failed to open or close a file descriptor.
 
### VFS

A [virtual file system](https://en.wikipedia.org/wiki/Virtual_file_system) (VFS) is an layer on top of regular
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

The collector configuration file follows the same structure as `netdata.conf`. It is divided in different sections, with
each one of them having the internal variables.

### `[global]`

In this section we define variables applied to the whole collector and the other subsections.

#### load

The collector has three different eBPF programs. These programs monitor the same functions inside the kernel, but they
monitor, process, and display different kinds of information.

By default, this plugin uses the `entry` mode. Changing this mode can create significant overhead on your operating
system, but also offer important information if you are developing or debugging software. The `load` option accepts the
following values: ​

-   `entry`: This is the default mode. In this mode, Netdata monitors only calls for the functions described in the
    sections above. When this mode is selected, Netdata does not show charts related to errors.
-   `return`: In this mode, Netdata also monitors the calls to function. In the `entry` mode, Netdata only traces kernel
    functions, but with `return`, Netdata also monitors the return of each function. This mode creates more charts, but
    also creates an overhead of roughly 110 nanosections for each function call.
