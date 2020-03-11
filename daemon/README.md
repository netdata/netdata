<!--
---
title: "Netdata daemon"
custom_edit_url: https://github.com/netdata/netdata/edit/master/daemon/README.md
---
-->

# Netdata daemon

## Starting netdata

-   You can start Netdata by executing it with `/usr/sbin/netdata` (the installer will also start it).

-   You can stop Netdata by killing it with `killall netdata`. You can stop and start Netdata at any point. When
    exiting, the [database engine](../database/engine/README.md) saves metrics to `/var/cache/netdata/dbengine/` so that
    it can continue when started again.

Access to the web site, for all graphs, is by default on port `19999`, so go to:

```sh
http://127.0.0.1:19999/
```

You can get the running config file at any time, by accessing `http://127.0.0.1:19999/netdata.conf`.

### Starting Netdata at boot

In the `system` directory you can find scripts and configurations for the
various distros.

#### systemd

The installer already installs `netdata.service` if it detects a systemd system.

To install `netdata.service` by hand, run:

```sh
# stop Netdata
killall netdata

# copy netdata.service to systemd
cp system/netdata.service /etc/systemd/system/

# let systemd know there is a new service
systemctl daemon-reload

# enable Netdata at boot
systemctl enable netdata

# start Netdata
systemctl start netdata
```

#### init.d

In the system directory you can find `netdata-lsb`. Copy it to the proper place according to your distribution
documentation. For Ubuntu, this can be done via running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-lsb /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
update-rc.d netdata defaults
```

#### openrc (gentoo)

In the `system` directory you can find `netdata-openrc`. Copy it to the proper
place according to your distribution documentation.

#### CentOS / Red Hat Enterprise Linux

For older versions of RHEL/CentOS that don't have systemd, an init script is included in the system directory. This can
be installed by running the following commands as root.

```sh
# copy the Netdata startup file to /etc/init.d
cp system/netdata-init-d /etc/init.d/netdata

# make sure it is executable
chmod +x /etc/init.d/netdata

# enable it
chkconfig --add netdata
```

_There have been some recent work on the init script, see PR
<https://github.com/netdata/netdata/pull/403>_

#### other systems

You can start Netdata by running it from `/etc/rc.local` or equivalent.

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

 Copyright (C) 2016-2017, Costa Tsaousis <costa@tsaousis.gr>
 Released under GNU General Public License v3 or later.
 All rights reserved.

 Home Page  : https://my-netdata.io
 Source Code: https://github.com/netdata/netdata
 Wiki / Docs: https://github.com/netdata/netdata/wiki
 Support    : https://github.com/netdata/netdata/issues
 License    : https://github.com/netdata/netdata/blob/master/LICENSE

 Twitter    : https://twitter.com/linuxnetdata
 Facebook   : https://www.facebook.com/linuxnetdata/


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

  -W simple-pattern pattern string
                           Check if string matches pattern and exit.


 Signals netdata handles:

  - HUP                    Close and reopen log files.
  - USR1                   Save internal DB to disk.
  - USR2                   Reload health configuration.
```

You can send commands during runtime via [netdatacli](../cli).

## Log files

Netdata uses 3 log files:

1.  `error.log`
2.  `access.log`
3.  `debug.log`

Any of them can be disabled by setting it to `/dev/null` or `none` in `netdata.conf`. By default `error.log` and
`access.log` are enabled. `debug.log` is only enabled if debugging/tracing is also enabled (Netdata needs to be compiled
with debugging enabled).

Log files are stored in `/var/log/netdata/` by default.

### error.log

The `error.log` is the `stderr` of the `netdata` daemon and all external plugins
run by `netdata`.

So if any process, in the Netdata process tree, writes anything to its standard error,
it will appear in `error.log`.

For most Netdata programs (including standard external plugins shipped by netdata), the following lines may appear:

| tag     | description                                                                                                               |
|:-:|:----------|
| `INFO`  | Something important the user should know.                                                                                 |
| `ERROR` | Something that might disable a part of netdata.<br/>The log line includes `errno` (if it is not zero).                    |
| `FATAL` | Something prevented a program from running.<br/>The log line includes `errno` (if it is not zero) and the program exited. |

So, when auto-detection of data collection fail, `ERROR` lines are logged and the relevant modules are disabled, but the
program continues to run.

When a Netdata program cannot run at all, a `FATAL` line is logged.

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

## OOM Score

Netdata runs with `OOMScore = 1000`. This means Netdata will be the first to be killed when your server runs out of
memory.

You can set Netdata OOMScore in `netdata.conf`, like this:

```conf
[global]
    OOM score = 1000
```

Netdata logs its OOM score when it starts:

```sh
# grep OOM /var/log/netdata/error.log
2017-10-15 03:47:31: netdata INFO : Adjusted my Out-Of-Memory (OOM) score from 0 to 1000.
```

### OOM score and systemd

Netdata will not be able to lower its OOM Score below zero, when it is started as the `netdata` user (systemd case).

To allow Netdata control its OOM Score in such cases, you will need to edit `netdata.service` and set:

```sh
[Service]
# The minimum Netdata Out-Of-Memory (OOM) score.
# Netdata (via [global].OOM score in netdata.conf) can only increase the value set here.
# To decrease it, set the minimum here and set the same or a higher value in netdata.conf.
# Valid values: -1000 (never kill netdata) to 1000 (always kill netdata).
OOMScoreAdjust=-1000
```

Run `systemctl daemon-reload` to reload these changes.

The above, sets and OOMScore for Netdata to `-1000`, so that Netdata can increase it via `netdata.conf`.

If you want to control it entirely via systemd, you can set in `netdata.conf`:

```conf
[global]
    OOM score = keep
```

Using the above, whatever OOM Score you have set at `netdata.service` will be maintained by netdata.

## Netdata process scheduling policy

By default Netdata runs with the `idle` process scheduling policy, so that it uses CPU resources, only when there is
idle CPU to spare. On very busy servers (or weak servers), this can lead to gaps on the charts.

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

### scheduling priority for `rr` and `fifo`

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

## scheduling settings and systemd

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

then execute this to restart netdata:

```sh
sudo service netdata restart
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
    the options supported at [log.h](../libnetdata/log/log.h). They are the `D_*` defines. The value
    `0xffffffffffffffff` will enable all possible debug flags.

Once Netdata is compiled with debugging and tracing is enabled for a few sections, the file `/var/log/netdata/debug.log`
will contain the messages.

> Do not forget to disable tracing (`debug flags = 0`) when you are done tracing. The file `debug.log` can grow too
> fast.

### compiling Netdata with debugging

To compile Netdata with debugging, use this:

```sh
# step into the Netdata source directory
cd /usr/src/netdata.git

# run the installer with debugging enabled
CFLAGS="-O1 -ggdb -DNETDATA_INTERNAL_CHECKS=1" ./netdata-installer.sh
```

The above will compile and install Netdata with debugging info embedded. You can now use `debug flags` to set the
section(s) you need to trace.

### debugging crashes

We have made the most to make Netdata crash free. If however, Netdata crashes on your system, it would be very helpful
to provide stack traces of the crash. Without them, is will be almost impossible to find the issue (the code base is
quite large to find such an issue by just objerving it).

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

### you can reproduce a Netdata crash on your system

> you need to have Netdata compiled with debugging info for this to work (check above)

Install the package `valgrind` and run:

```sh
valgrind $(which netdata) -D
```

Netdata will start and it will be a lot slower. Now reproduce the crash and `valgrind` will dump on your console the
stack trace. Open a new github issue and post the output.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdaemon%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
