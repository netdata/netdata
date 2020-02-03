# ebpf_process.plugin

This plugin uses eBPF to monitor some system calls inside the kernel, for while, the main goal of this plugin is to monitor
IO and process management on the host where it is running.

This plugin has different configuration modes that can be adjusted with its file called `ebpf_process.conf`. It always starts
using the less expensive mode (`entry`) that is useful to have a general vision about what the software are doing on the
computer, but you can change according your necessity (`/etc/netdata/edit-config ebpf_process.conf`).

## Linux configuration

This collector only works on Linux, but there is already a work to bring eBPF for FreeBSD. 

To run the collector on Linux distribution the kernel must be at least the version `4.11.0` and the kernel needs to compiled
with the option `CONFIG_KPROBES=y`.

Another requirement to run this plugin is to have the `tracefs` and `debugfs` mounted on your system.

### Compiling Kernel

The first step before to run the collector is to enable `kprobes` on Linux, you can verify whether your kernel has this option
enabled running the following command:

```
# zgrep CONFIG_KPROBES=y /proc/config.gz
CONFIG_KPROBES=y
```

case you have an output different of the previous, it will be necessary to recompile your kernel and enable it. The next 
instructions to compile the kernel assumes that you already have a `.config` file inside your kernel source, case this is
not your case, you will need to do more steps to compile your kernel.

-   Move to the kernel source directory: 
    -   ```# cd /usr/src/linux```
-   Open the menu to configure the kernel:
    -   ```# make menuconfig ```
-   This step depends of your kernel version, we want to enable the option `Kprobes`, it can be present inside one of 
the menu options:
    -   `General setup`: For old kernels
    -   `General architecture-dependent options`: More recent kernels
-   Use the `Save` option to store the changes inside `.config`.
-   Exit the menu with the option `Exit`.
-   Compile your kernel running
    -   ```# make bzImage```
-   Copy your kernel to the directory configured inside your boot loader, for example, case you are using elilo the destination
could be:
    -    ```# cp arch/x86_64/boot/bzImage /boot/efi/EFI/Linux/`    
-   Case you are using efi this would be sufficient, but for others boot loaders it will be necessary to execute other steps
before to reboot your computer.    

### Mount `debugfs` and `tracefs`

Case your distribution is not mouting the `tracefs` and `debugfs` filesystems, it will be necessary to mount them either 
using command line

```
# mount -t debugfs nodev /sys/kernel/debug
# mount -t tracefs nodev /sys/kernel/tracing
```

or configuring your `/etc/fstab`. 

## Charts

The first version of `ebpf_process.plugin` gives a general vision about process running on computer. The charts related to
this plugin are inside `eBPF` option on dashboard menu, this option is divided in three groups `file`, `vfs` and 
`process`.

All the collector charts demonstrate values per second, the total value is kept inside the collector and the eBPF program,
but we only draw the difference between the previous moment and current time.

### File

This group has two charts to demonstrate how the software are interacting with the Linux kernel to open and close file 
descriptors.

#### File descriptor

This chart contain two dimensions that demonstrates the number of calls to the functions `do_sys_open` and `__close_fd`. 
These functions are not commonly called from software, but they are behind the system cals `open(2)`, `openat(2)` and
 `close(2)`.

#### File error

This charts demonstrate the number of times that there was an error to try to open or close a file descriptor on the
 Operate System,
 
### VFS

Virtual Function is a layer above file systems, the functions present inside this API are not obligatory for all file systems,
so it possible that the charts in this group won't demonstrate all actions happened on your computer.

#### Deleted objects

This chart monitors calls for `vfs_unlink`, this function is responsible to remove an object from the file system. 

#### IO

On this chart Netdata demonstrates the number of calls to the functions `vfs_read` and `vfs_write`.

#### IO Bytes

This is another chart that monitor the functions `vfs_read` and `vfs_write`, but instead to show the number of calls, it 
 shows the total of bytes read and written with these functions.
 
We demonstrate the number of bytes written as negative, because they are moving down to disk.
 
#### IO Errors

When there is an error to read or write information on a file system, Netdata counts these events and do not count the total
of bytes given as argument to the functions. 

### Process

On this group of charts Netdata is monitoring the process/thread creation and process end, here we also monitor possible
errors when some of these actions happened. 
 
#### Process Thread

Internally the linux Kernel treats both process and thread as `tasks`. To create a thread the Linux give us different
system calls (`fork(2)`, `vfork(2)` and `clone(2)`), but these system calls will call only one function given different
arguments to it, the function `_do_fork`. To generate this chart Netdata monitors `_do_fork` to populate the dimension
`process` and it also monitors `sys_clone` to identify the threads.

#### Exit


## Configuration
