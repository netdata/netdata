# Netdata daemon



## Command line options

Normally you don't need to supply any command line arguments to netdata.

If you do though, they override the configuration equivalent options.

To get a list of all command line parameters supported, run:

```sh
netdata -h
```

The program will print the supported command line parameters.

The command line options of the netdata 1.10.0 version are the following:
```

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
 License    : https://github.com/netdata/netdata/blob/master/LICENSE.md

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

  -W set section option value
                           set netdata.conf option from the command line.

  -W simple-pattern pattern string
                           Check if string matches pattern and exit.


 Signals netdata handles:

  - HUP                    Close and reopen log files.
  - USR1                   Save internal DB to disk.
  - USR2                   Reload health configuration.
```

## Log files

netdata uses 3 log files:

1. `error.log`
2. `access.log`
3. `debug.log`

Any of them can be disabled by setting it to `/dev/null` or `none` in `netdata.conf`.
By default `error.log` and `access.log` are enabled. `debug.log` is only enabled if
debugging/tracing is also enabled (netdata needs to be compiled with debugging enabled).

Log files are stored in `/var/log/netdata/` by default.

#### error.log

The `error.log` is the `stderr` of the netdata daemon and all external plugins run by netdata.

So if any process, in the netdata process tree, writes anything to its standard error,
it will appear in `error.log`.

For most netdata programs (including standard external plugins shipped by netdata), the
following lines may appear:

tag|description
:---:|:----
`INFO`|Something important the user should know.
`ERROR`|Something that might disable a part of netdata.<br/>The log line includes `errno` (if it is not zero).
`FATAL`|Something prevented a program from running.<br/>The log line includes `errno` (if it is not zero) and the program exited.

So, when auto-detection of data collection fail, `ERROR` lines are logged and the relevant modules
are disabled, but the program continues to run.

When a netdata program cannot run at all, a `FATAL` line is logged.

#### access.log

The `access.log` logs web requests. The format is:

```txt
DATE: ID: (sent/all = SENT_BYTES/ALL_BYTES bytes PERCENT_COMPRESSION%, prep/sent/total PREP_TIME/SENT_TIME/TOTAL_TIME ms): ACTION CODE URL
```

where:

 - `ID` is the client ID. Client IDs are auto-incremented every time a client connects to netdata.
 - `SENT_BYTES` is the number of bytes sent to the client, without the HTTP response header.
 - `ALL_BYTES` is the number of bytes of the response, before compression.
 - `PERCENT_COMPRESSION` is the percentage of traffic saved due to compression.
 - `PREP_TIME` is the time in milliseconds needed to prepared the response.
 - `SENT_TIME` is the time in milliseconds needed to sent the response to the client.
 - `TOTAL_TIME` is the total time the request was inside netdata (from the first byte of the request to the last byte of the response).
 - `ACTION` can be `filecopy`, `options` (used in CORS), `data` (API call).


#### debug.log

See [debugging](#debugging).


## OOM Score

netdata runs with `OOMScore = 1000`. This means netdata will be the first to be killed when your
server runs out of memory.

You can set netdata OOMScore in `netdata.conf`, like this:

```
[global]
    OOM score = 1000
```

netdata logs its OOM score when it starts:

```sh
# grep OOM /var/log/netdata/error.log
2017-10-15 03:47:31: netdata INFO : Adjusted my Out-Of-Memory (OOM) score from 0 to 1000.
```

#### OOM score and systemd

netdata will not be able to lower its OOM Score below zero, when it is started as the `netdata`
user (systemd case).

To allow netdata control its OOM Score in such cases, you will need to edit
`netdata.service` and set:

```
[Service]
# The minimum netdata Out-Of-Memory (OOM) score.
# netdata (via [global].OOM score in netdata.conf) can only increase the value set here.
# To decrease it, set the minimum here and set the same or a higher value in netdata.conf.
# Valid values: -1000 (never kill netdata) to 1000 (always kill netdata).
OOMScoreAdjust=-1000
```

Run `systemctl daemon-reload` to reload these changes.

The above, sets and OOMScore for netdata to `-1000`, so that netdata can increase it via
`netdata.conf`.

If you want to control it entirely via systemd, you can set in `netdata.conf`:

```
[global]
    OOM score = keep
```

Using the above, whatever OOM Score you have set at `netdata.service` will be maintained by netdata.


## netdata process scheduling policy

By default netdata runs with the `idle` process scheduling policy, so that it uses CPU resources, only when there is idle CPU to spare. On very busy servers (or weak servers), this can lead to gaps on the charts.

You can set netdata scheduling policy in `netdata.conf`, like this:

```
[global]
  process scheduling policy = idle
```

You can use the following:

policy|description
:-----:|:--------
`idle`|use CPU only when there is spare - this is lower than nice 19 - it is the default for netdata and it is so low that netdata will run in "slow motion" under extreme system load, resulting in short (1-2 seconds) gaps at the charts.
`other`<br/>or<br/>`nice`|this is the default policy for all processes under Linux. It provides dynamic priorities based on the `nice` level of each process. Check below for setting this `nice` level for netdata.
`batch`|This policy is similar to `other` in that it schedules the thread according to its dynamic priority (based on the `nice` value).  The difference is that this policy will cause the scheduler to always assume that the thread is CPU-intensive.  Consequently, the scheduler will  apply a small scheduling penalty with respect to wake-up behavior, so that this thread is mildly disfavored in scheduling decisions.
`fifo`|`fifo` can be used only with static priorities higher than 0, which means that when a `fifo` threads becomes runnable, it will always  immediately  preempt  any  currently running  `other`, `batch`, or `idle` thread.  `fifo` is a simple scheduling algorithm without time slicing.
`rr`|a simple enhancement of `fifo`.  Everything described above for `fifo` also applies to `rr`, except that each thread is allowed to run only for a  maximum time quantum.
`keep`<br/>or<br/>`none`|do not set scheduling policy, priority or nice level - i.e. keep running with whatever it is set already (e.g. by systemd).

For more information see `man sched`.

### scheduling priority for `rr` and `fifo`

Once the policy is set to one of `rr` or `fifo`, the following will appear:

```
[global]
    process scheduling priority = 0
```

These priorities are usually from 0 to 99. Higher numbers make the process more important.

### nice level for policies `other` or `batch`

When the policy is set to `other`, `nice`, or `batch`, the following will appear:

```
[global]
    process nice level = 19
```

## scheduling settings and systemd

netdata will not be able to set its scheduling policy and priority to more important values when it is started as the `netdata` user (systemd case).

You can set these settings at `/etc/systemd/system/netdata.service`:

```
[Service]
# By default netdata switches to scheduling policy idle, which makes it use CPU, only
# when there is spare available.
# Valid policies: other (the system default) | batch | idle | fifo | rr
#CPUSchedulingPolicy=other

# This sets the maximum scheduling priority netdata can set (for policies: rr and fifo).
# netdata (via [global].process scheduling priority in netdata.conf) can only lower this value.
# Priority gets values 1 (lowest) to 99 (highest).
#CPUSchedulingPriority=1

# For scheduling policy 'other' and 'batch', this sets the lowest niceness of netdata.
# netdata (via [global].process nice level in netdata.conf) can only increase the value set here.
#Nice=0
```

Run `systemctl daemon-reload` to reload these changes.

Now, tell netdata to keep these settings, as set by systemd, by editing `netdata.conf` and setting:

```
[global]
    process scheduling policy = keep
```

Using the above, whatever scheduling settings you have set at `netdata.service` will be maintained by netdata.


#### Example 1: netdata with nice -1 on non-systemd systems

On a system that is not based on systemd, to make netdata run with nice level -1 (a little bit higher to the default for all programs), edit netdata.conf and set:

```
[global]
  process scheduling policy = other
  process nice level = -1
```

then execute this to restart netdata:

```sh
sudo service netdata restart
```


#### Example 2: netdata with nice -1 on systemd systems

On a system that is based on systemd, to make netdata run with nice level -1 (a little bit higher to the default for all programs), edit netdata.conf and set:

```
[global]
  process scheduling policy = keep
```

edit /etc/systemd/system/netdata.service and set:

```
[Service]
CPUSchedulingPolicy=other
Nice=-1
```

then execute:

```sh
sudo systemctl daemon-reload
sudo systemctl restart netdata
```

## virtual memory

You may notice that netdata's virtual memory size, as reported by `ps` or `/proc/pid/status`
(or even netdata's applications virtual memory chart)  is unrealistically high.

For example, it may be reported to be 150+MB, even if the resident memory size is just 25MB.
Similar values may be reported for netdata plugins too.

Check this for example: A netdata installation with default settings on Ubuntu 16.04LTS.
The top chart is **real memory used**, while the bottom one is **virtual memory**:

![image](https://cloud.githubusercontent.com/assets/2662304/19013772/5eb7173e-87e3-11e6-8f2b-a2ccfeb06faf.png)

#### why this happens?

The system memory allocator allocates virtual memory arenas, per thread running.
On Linux systems this defaults to 16MB per thread on 64 bit machines. So, if you get the
difference between real and virtual memory and divide it by 16MB you will roughly get the
number of threads running.

The system does this for speed. Having a separate memory arena for each thread, allows the
threads to run in parallel in multi-core systems, without any locks between them.

This behaviour is system specific. For example, the chart above when running netdata on alpine
linux (that uses **musl** instead of **glibc**) is this:

![image](https://cloud.githubusercontent.com/assets/2662304/19013807/7cf5878e-87e4-11e6-9651-082e68701eab.png)

#### can we do anything to lower it?

Since netdata already uses minimal memory allocations while it runs (i.e. it adapts its memory
on start, so that while repeatedly collects data it does not do memory allocations), it already
instructs the system memory allocator to minimize the memory arenas for each thread. We have also
added [2 configuration options](https://github.com/netdata/netdata/blob/5645b1ee35248d94e6931b64a8688f7f0d865ec6/src/main.c#L410-L418)
to allow you tweak these settings.

However, even if we instructed the memory allocator to use just one arena, it seems it allocates
an arena per thread.

netdata also supports `jemalloc` and `tcmalloc`, however both behave exactly the same to the
glibc memory allocator in this aspect.

#### Is this a problem?

No, it is not.

Linux reserves real memory (physical RAM) in pages (on x86 machines pages are 4KB each).
So even if the system memory allocator is allocating huge amounts of virtual memory,
only the 4KB pages that are actually used are reserving physical RAM. The **real memory** chart
on netdata application section, shows the amount of physical memory these pages occupy(it
accounts the whole pages, even if parts of them are actually used).


## Debugging

When you compile netdata with debugging:

1. compiler optimizations for your CPU are disabled (netdata will run somewhat slower)

2. a lot of code is added all over netdata, to log debug messages to `/var/log/netdata/debug.log`. However, nothing is printed by default. netdata allows you to select which sections of netdata you want to trace. Tracing is activated via the config option `debug flags`. It accepts a hex number, to enable or disable specific sections. You can find the options supported at [log.h](https://github.com/netdata/netdata/blob/master/libnetdata/log/log.h). They are the `D_*` defines. The value `0xffffffffffffffff` will enable all possible debug flags.

Once netdata is compiled with debugging and tracing is enabled for a few sections, the file `/var/log/netdata/debug.log` will contain the messages.

> Do not forget to disable tracing (`debug flags = 0`) when you are done tracing. The file `debug.log` can grow too fast.

#### compiling netdata with debugging

To compile netdata with debugging, use this:

```sh
# step into the netdata source directory
cd /usr/src/netdata.git

# run the installer with debugging enabled
CFLAGS="-O1 -ggdb -DNETDATA_INTERNAL_CHECKS=1" ./netdata-installer.sh
```

The above will compile and install netdata with debugging info embedded. You can now use `debug flags` to set the section(s) you need to trace.

#### debugging crashes

We have made the most to make netdata crash free. If however, netdata crashes on your system, it would be very helpful to provide stack traces of the crash. Without them, is will be almost impossible to find the issue (the code base is quite large to find such an issue by just objerving it).

To provide stack traces, **you need to have netdata compiled with debugging**. There is no need to enable any tracing (`debug flags`).

Then you need to be in one of the following 2 cases:

1. netdata crashes and you have a core dump

2. you can reproduce the crash

If you are not on these cases, you need to find a way to be (i.e. if your system does not produce core dumps, check your distro documentation to enable them).

#### netdata crashes and you have a core dump

> you need to have netdata compiled with debugging info for this to work (check above)

Run the following command and post the output on a github issue.

```sh
gdb $(which netdata) /path/to/core/dump
```

#### you can reproduce a netdata crash on your system

> you need to have netdata compiled with debugging info for this to work (check above)

Install the package `valgrind` and run:

```sh
valgrind $(which netdata) -D
```

netdata will start and it will be a lot slower. Now reproduce the crash and `valgrind` will dump on your console the stack trace. Open a new github issue and post the output.

