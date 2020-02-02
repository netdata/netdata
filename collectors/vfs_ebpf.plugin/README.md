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

The first version of `ebpf_process.plugin` gives a general vision about process running on computer, it brings 
