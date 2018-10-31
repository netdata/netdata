# Tracing Options

When you compile netdata with debugging:

1. compiler optimizations for your CPU are disabled (netdata will run somewhat slower)

2. a lot of code is added all over netdata, to log debug messages to `/var/log/netdata/debug.log`. However, nothing is printed by default. netdata allows you to select which sections of netdata you want to trace. Tracing is activated via the config option `debug flags`. It accepts a hex number, to enable or disable specific sections. You can find the options supported at [log.h](https://github.com/netdata/netdata/blob/master/src/log.h). They are the `D_*` defines. The value `0xffffffffffffffff` will enable all possible debug flags.

Once netdata is compiled with debugging and tracing is enabled for a few sections, the file `/var/log/netdata/debug.log` will contain the messages.

> Do not forget to disable tracing (`debug flags = 0`) when you are done tracing. The file `debug.log` can grow too fast.

## compiling netdata with debugging

To compile netdata with debugging, use this:

```sh
# step into the netdata source directory
cd /usr/src/netdata.git

# run the installer with debugging enabled
CFLAGS="-O1 -ggdb -DNETDATA_INTERNAL_CHECKS=1" ./netdata-installer.sh
```

The above will compile and install netdata with debugging info embedded. You can now use `debug flags` to set the section(s) you need to trace.

## debugging crashes

We have made the most to make netdata crash free. If however, netdata crashes on your system, it would be very helpful to provide stack traces of the crash. Without them, is will be almost impossible to find the issue (the code base is quite large to find such an issue by just objerving it).

To provide stack traces, **you need to have netdata compiled with debugging**. There is no need to enable any tracing (`debug flags`).

Then you need to be in one of the following 2 cases:

1. netdata crashes and you have a core dump

2. you can reproduce the crash

If you are not on these cases, you need to find a way to be (i.e. if your system does not produce core dumps, check your distro documentation to enable them).

## netdata crashes and you have a core dump

> you need to have netdata compiled with debugging info for this to work (check above)

Run the following command and post the output on a github issue.

```sh
gdb $(which netdata) /path/to/core/dump
```

## you can reproduce a netdata crash on your system

> you need to have netdata compiled with debugging info for this to work (check above)

Install the package `valgrind` and run:

```sh
valgrind $(which netdata) -D
```

netdata will start and it will be a lot slower. Now reproduce the crash and `valgrind` will dump on your console the stack trace. Open a new github issue and post the output.
