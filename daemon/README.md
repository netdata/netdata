# Netdata daemon

The Netdata daemon is practically a synonym for the Netdata Agent, as it controls its 
entire operation. We support various methods to 
[start, stop, or restart the daemon](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md).

This document provides some basic information on the command line options, log files, and how to debug and troubleshoot

## Command line options

Normally you don't need to supply any command line arguments to netdata.

If you do though, they override the configuration equivalent options.

To get a list of all command line parameters supported, run:

```sh
netdata -h
```

The program will print the supported command line parameters.

The command line options of the Netdata 1.10.0 version are the following:

```sh
 ^
 |.-.   .-.   .-.   .-.   .  netdata                                         
 |   '-'   '-'   '-'   '-'   real-time performance monitoring, done right!   
 +----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+-----+--->

 Copyright (C) 2016-2022, Netdata, Inc. <info@netdata.cloud>
 Released under GNU General Public License v3 or later.
 All rights reserved.

 Home Page  : https://netdata.cloud
 Source Code: https://github.com/netdata/netdata
 Docs       : https://learn.netdata.cloud
 Support    : https://github.com/netdata/netdata/issues
 License    : https://github.com/netdata/netdata/blob/master/LICENSE.md

 Twitter    : https://twitter.com/linuxnetdata
 LinkedIn   : https://linkedin.com/company/netdata-cloud/
 Facebook   : https://facebook.com/linuxnetdata/


 SYNOPSIS: netdata [options]

 Options:

  -c filename              Configuration file to load.
                           Default: /etc/netdata/netdata.conf

  -D                       Do not fork. Run in the foreground.
                           Default: run in the background

  -h                       Display this help message.

  -P filename              File to save a pid while running.
                           Default: do not save pid to a file

  -i IP                    The IP address to listen to.
                           Default: all IP addresses IPv4 and IPv6

  -p port                  API/Web port to use.
                           Default: 19999

  -s path                  Prefix for /proc and /sys (for containers).
                           Default: no prefix

  -t seconds               The internal clock of netdata.
                           Default: 1

  -u username              Run as user.
                           Default: netdata

  -v                       Print netdata version and exit.

  -V                       Print netdata version and exit.

  -W options               See Advanced options below.


 Advanced options:

  -W stacksize=N           Set the stacksize (in bytes).

  -W debug_flags=N         Set runtime tracing to debug.log.

  -W unittest              Run internal unittests and exit.

  -W createdataset=N       Create a DB engine dataset of N seconds and exit.

  -W set section option value
                           set netdata.conf option from the command line.

  -W buildinfo             Print the version, the configure options, 
                           a list of optional features, and whether they 
                           are enabled or not.

  -W buildinfojson         Print the version, the configure options, 
                           a list of optional features, and whether they 
                           are enabled or not, in JSON format.
  
  -W simple-pattern pattern string
                           Check if string matches pattern and exit.

  -W "claim -token=TOKEN -rooms=ROOM1,ROOM2 url=https://api.netdata.cloud"
                           Connect the agent to the workspace rooms pointed to by TOKEN and ROOM*.

 Signals netdata handles:

  - HUP                    Close and reopen log files.
  - USR1                   Save internal DB to disk.
  - USR2                   Reload health configuration.
```

You can send commands during runtime via [netdatacli](https://github.com/netdata/netdata/blob/master/cli/README.md).

## Log files

Netdata uses 4 log files:

1.  `error.log`
2.  `collector.log`
3.  `access.log`
4.  `debug.log`

Any of them can be disabled by setting it to `/dev/null` or `none` in `netdata.conf`. By default `error.log`,
`collector.log`, and `access.log` are enabled. `debug.log` is only enabled if debugging/tracing is also enabled
(Netdata needs to be compiled with debugging enabled).

Log files are stored in `/var/log/netdata/` by default.

### error.log

The `error.log` is the `stderr` of the `netdata` daemon .

For most Netdata programs (including standard external plugins shipped by netdata), the following lines may appear:

| tag     | description                                                                                                               |
|:-:|:----------|
| `INFO`  | Something important the user should know.                                                                                 |
| `ERROR` | Something that might disable a part of netdata.<br/>The log line includes `errno` (if it is not zero).                    |
| `FATAL` | Something prevented a program from running.<br/>The log line includes `errno` (if it is not zero) and the program exited. |

So, when auto-detection of data collection fail, `ERROR` lines are logged and the relevant modules are disabled, but the
program continues to run.

When a Netdata program cannot run at all, a `FATAL` line is logged.

### collector.log

The `collector.log` is the `stderr` of all [collectors](https://github.com/netdata/netdata/blob/master/collectors/COLLECTORS.md)
 run by `netdata`.

So if any process, in the Netdata process tree, writes anything to its standard error,
it will appear in `collector.log`.

Data stored inside this file follows pattern already described for `error.log`.

### access.log

The `access.log` logs web requests. The format is:

```txt
DATE: ID: (sent/all = SENT_BYTES/ALL_BYTES bytes PERCENT_COMPRESSION%, prep/sent/total PREP_TIME/SENT_TIME/TOTAL_TIME ms): ACTION CODE URL
```

where:

-   `ID` is the client ID. Client IDs are auto-incremented every time a client connects to netdata.
-   `SENT_BYTES` is the number of bytes sent to the client, without the HTTP response header.
-   `ALL_BYTES` is the number of bytes of the response, before compression.
-   `PERCENT_COMPRESSION` is the percentage of traffic saved due to compression.
-   `PREP_TIME` is the time in milliseconds needed to prepared the response.
-   `SENT_TIME` is the time in milliseconds needed to sent the response to the client.
-   `TOTAL_TIME` is the total time the request was inside Netdata (from the first byte of the request to the last byte
    of the response).
-   `ACTION` can be `filecopy`, `options` (used in CORS), `data` (API call).

### debug.log

See [debugging](#debugging).

## Netdata process scheduling policy

By default Netdata versions prior to 1.34.0 run with the `idle` process scheduling policy, so that it uses CPU
resources, only when there is idle CPU to spare. On very busy servers (or weak servers), this can lead to gaps on
the charts.

Starting with version 1.34.0, Netdata instead uses the `batch` scheduling policy by default. This largely eliminates
issues with gaps in charts on busy systems while still keeping the impact on the rest of the system low.

You can set Netdata scheduling policy in `netdata.conf`, like this:

```conf
[global]
  process scheduling policy = idle
```

You can use the following:

| policy                    | description                                                                                                                                                                                                                                                                                                                                                                                                              |
| :-----------------------: | :----------                                                                                                                                                                                                                                                                                                                                                                                                              |
| `idle`                    | use CPU only when there is spare - this is lower than nice 19 - it is the default for Netdata and it is so low that Netdata will run in "slow motion" under extreme system load, resulting in short (1-2 seconds) gaps at the charts.                                                                                                                                                                                    |
| `other`<br/>or<br/>`nice` | this is the default policy for all processes under Linux. It provides dynamic priorities based on the `nice` level of each process. Check below for setting this `nice` level for netdata.                                                                                                                                                                                                                               |
| `batch`                   | This policy is similar to `other` in that it schedules the thread according to its dynamic priority (based on the `nice` value).  The difference is that this policy will cause the scheduler to always assume that the thread is CPU-intensive.  Consequently, the scheduler will  apply a small scheduling penalty with respect to wake-up behavior, so that this thread is mildly disfavored in scheduling decisions. |
| `fifo`                    | `fifo` can be used only with static priorities higher than 0, which means that when a `fifo` threads becomes runnable, it will always  immediately  preempt  any  currently running  `other`, `batch`, or `idle` thread.  `fifo` is a simple scheduling algorithm without time slicing.                                                                                                                                  |
| `rr`                      | a simple enhancement of `fifo`.  Everything described above for `fifo` also applies to `rr`, except that each thread is allowed to run only for a  maximum time quantum.                                                                                                                                                                                                                                                 |
| `keep`<br/>or<br/>`none`  | do not set scheduling policy, priority or nice level - i.e. keep running with whatever it is set already (e.g. by systemd).                                                                                                                                                                                                                                                                                              |

For more information see `man sched`.

### Scheduling priority for `rr` and `fifo`

Once the policy is set to one of `rr` or `fifo`, the following will appear:

```conf
[global]
    process scheduling priority = 0
```

These priorities are usually from 0 to 99. Higher numbers make the process more
important.

### nice level for policies `other` or `batch`

When the policy is set to `other`, `nice`, or `batch`, the following will appear:

```conf
[global]
    process nice level = 19
```

## Scheduling settings and systemd

Netdata will not be able to set its scheduling policy and priority to more important values when it is started as the
`netdata` user (systemd case).

You can set these settings at `/etc/systemd/system/netdata.service`:

```sh
[Service]
# By default Netdata switches to scheduling policy idle, which makes it use CPU, only
# when there is spare available.
# Valid policies: other (the system default) | batch | idle | fifo | rr
#CPUSchedulingPolicy=other

# This sets the maximum scheduling priority Netdata can set (for policies: rr and fifo).
# Netdata (via [global].process scheduling priority in netdata.conf) can only lower this value.
# Priority gets values 1 (lowest) to 99 (highest).
#CPUSchedulingPriority=1

# For scheduling policy 'other' and 'batch', this sets the lowest niceness of netdata.
# Netdata (via [global].process nice level in netdata.conf) can only increase the value set here.
#Nice=0
```

Run `systemctl daemon-reload` to reload these changes.

Now, tell Netdata to keep these settings, as set by systemd, by editing
`netdata.conf` and setting:

```conf
[global]
    process scheduling policy = keep
```

Using the above, whatever scheduling settings you have set at `netdata.service`
will be maintained by netdata.

### Example 1: Netdata with nice -1 on non-systemd systems

On a system that is not based on systemd, to make Netdata run with nice level -1 (a little bit higher to the default for
all programs), edit `netdata.conf` and set:

```conf
[global]
  process scheduling policy = other
  process nice level = -1
```

then execute this to [restart Netdata](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md):

```sh
sudo systemctl restart netdata
```

#### Example 2: Netdata with nice -1 on systemd systems

On a system that is based on systemd, to make Netdata run with nice level -1 (a little bit higher to the default for all
programs), edit `netdata.conf` and set:

```conf
[global]
  process scheduling policy = keep
```

edit /etc/systemd/system/netdata.service and set:

```sh
[Service]
CPUSchedulingPolicy=other
Nice=-1
```

then execute:

```sh
sudo systemctl daemon-reload
sudo systemctl restart netdata
```

## Virtual memory

You may notice that netdata's virtual memory size, as reported by `ps` or `/proc/pid/status` (or even netdata's
applications virtual memory chart) is unrealistically high.

For example, it may be reported to be 150+MB, even if the resident memory size is just 25MB. Similar values may be
reported for Netdata plugins too.

Check this for example: A Netdata installation with default settings on Ubuntu
16.04LTS. The top chart is **real memory used**, while the bottom one is
**virtual memory**:

![image](https://cloud.githubusercontent.com/assets/2662304/19013772/5eb7173e-87e3-11e6-8f2b-a2ccfeb06faf.png)

### Why does this happen?

The system memory allocator allocates virtual memory arenas, per thread running. On Linux systems this defaults to 16MB
per thread on 64 bit machines. So, if you get the difference between real and virtual memory and divide it by 16MB you
will roughly get the number of threads running.

The system does this for speed. Having a separate memory arena for each thread, allows the threads to run in parallel in
multi-core systems, without any locks between them.

This behaviour is system specific. For example, the chart above when running
Netdata on Alpine Linux (that uses **musl** instead of **glibc**) is this:

![image](https://cloud.githubusercontent.com/assets/2662304/19013807/7cf5878e-87e4-11e6-9651-082e68701eab.png)

### Can we do anything to lower it?

Since Netdata already uses minimal memory allocations while it runs (i.e. it adapts its memory on start, so that while
repeatedly collects data it does not do memory allocations), it already instructs the system memory allocator to
minimize the memory arenas for each thread. We have also added [2 configuration
options](https://github.com/netdata/netdata/blob/5645b1ee35248d94e6931b64a8688f7f0d865ec6/src/main.c#L410-L418) to allow
you tweak these settings: `glibc malloc arena max for plugins` and `glibc malloc arena max for netdata`.

However, even if we instructed the memory allocator to use just one arena, it
seems it allocates an arena per thread.

Netdata also supports `jemalloc` and `tcmalloc`, however both behave exactly the
same to the glibc memory allocator in this aspect.

### Is this a problem?

No, it is not.

Linux reserves real memory (physical RAM) in pages (on x86 machines pages are 4KB each). So even if the system memory
allocator is allocating huge amounts of virtual memory, only the 4KB pages that are actually used are reserving physical
RAM. The **real memory** chart on Netdata application section, shows the amount of physical memory these pages occupy(it
accounts the whole pages, even if parts of them are actually used).

## Debugging

When you compile Netdata with debugging:

1.  compiler optimizations for your CPU are disabled (Netdata will run somewhat slower)

2.  a lot of code is added all over netdata, to log debug messages to `/var/log/netdata/debug.log`. However, nothing is
    printed by default. Netdata allows you to select which sections of Netdata you want to trace. Tracing is activated
    via the config option `debug flags`. It accepts a hex number, to enable or disable specific sections. You can find
    the options supported at [log.h](https://raw.githubusercontent.com/netdata/netdata/master/libnetdata/log/log.h).
    They are the `D_*` defines. The value `0xffffffffffffffff` will enable all possible debug flags.

Once Netdata is compiled with debugging and tracing is enabled for a few sections, the file `/var/log/netdata/debug.log`
will contain the messages.

> Do not forget to disable tracing (`debug flags = 0`) when you are done tracing. The file `debug.log` can grow too
> fast.

### Compiling Netdata with debugging

To compile Netdata with debugging, use this:

```sh
# step into the Netdata source directory
cd /usr/src/netdata.git

# run the installer with debugging enabled
CFLAGS="-O1 -ggdb -DNETDATA_INTERNAL_CHECKS=1" ./netdata-installer.sh
```

The above will compile and install Netdata with debugging info embedded. You can now use `debug flags` to set the
section(s) you need to trace.

### Debugging crashes

We have made the most to make Netdata crash free. If however, Netdata crashes on your system, it would be very helpful
to provide stack traces of the crash. Without them, is will be almost impossible to find the issue (the code base is
quite large to find such an issue by just observing it).

To provide stack traces, **you need to have Netdata compiled with debugging**. There is no need to enable any tracing
(`debug flags`).

Then you need to be in one of the following 2 cases:

1.  Netdata crashes and you have a core dump

2.  you can reproduce the crash

If you are not on these cases, you need to find a way to be (i.e. if your system does not produce core dumps, check your
distro documentation to enable them).

### Netdata crashes and you have a core dump

> you need to have Netdata compiled with debugging info for this to work (check above)

Run the following command and post the output on a github issue.

```sh
gdb $(which netdata) /path/to/core/dump
```

### You can reproduce a Netdata crash on your system

> you need to have Netdata compiled with debugging info for this to work (check above)

Install the package `valgrind` and run:

```sh
valgrind $(which netdata) -D
```

Netdata will start and it will be a lot slower. Now reproduce the crash and `valgrind` will dump on your console the
stack trace. Open a new github issue and post the output.
