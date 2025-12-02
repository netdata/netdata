# Daemon

The Netdata Daemon, often referred to as the Netdata Agent, controls the entire operation of the monitoring system. This document provides an overview of command-line options, debugging, and troubleshooting.

## Command-Line Options

While Netdata typically runs with default settings, you can override configurations using command-line options. For a complete list of options and detailed descriptions, run:

```sh
netdata -h
```

**Common Options**:

| Option        | Description                       | Default                     |
|---------------|-----------------------------------|-----------------------------|
| `-c filename` | Specify configuration file        | `/etc/netdata/netdata.conf` |
| `-D`          | Run in foreground (do not fork)   | Run in background           |
| `-d`          | Run in background (fork)          | Run in background           |
| `-P filename` | Save PID to file                  | No PID file                 |
| `-i IP`       | Set listening IP address          | All IPv4 and IPv6 addresses |
| `-p port`     | Set API/Web port                  | `19999`                     |
| `-s path`     | Set prefix for `/proc` and `/sys` | No prefix                   |
| `-t seconds`  | Set internal clock interval       | `1`                         |
| `-u username` | Set running user                  | `netdata`                   |
| `-v`, `-V`    | Display version and exit          | -                           |
| `-W options`  | Advanced options (see below)      | -                           |

## Logging

For details about Netdata's logging system and configuration, see [Netdata Logging](/src/libnetdata/log/README.md).

## Process Scheduling Policy (Unix only)

Netdata uses the `batch` scheduling policy by default, which helps eliminate gaps in charts on busy systems while maintaining low system impact.


<details>
<summary>Change (Systemd)</summary>

When Netdata runs under systemd as the `netdata` user, it can’t directly modify its scheduling policy and priority. Instead, configure these settings through systemd.

1. Use the following command to edit the systemd service (requires root privileges):

   ```bash
   systemctl edit netdata
   ```

2. Below are the available scheduling options. Uncomment and adjust the values according to your needs:

   ```bash
   [Service]
   ## CPU Scheduling Policy
   ## Options: other (system default) | batch | idle | fifo | rr
   #CPUSchedulingPolicy=other

   ## CPU Scheduling Priority (for fifo and rr policies)
   ## Range: 1 (lowest) to 99 (highest)
   ## Note: Netdata can only reduce this value via netdata.conf
   #CPUSchedulingPriority=1

   ## Process Nice Level (for other and batch policies)
   ## Range: -20 (highest) to 19 (lowest)
   ## Note: Netdata can only increase this value via netdata.conf
   #Nice=0
   ```

3. Configure Netdata to preserve systemd settings by editing `netdata.conf`:
      ```text
         [global]
            process scheduling policy = keep
      ```

4. [Restart](/docs/netdata-agent/start-stop-restart.md) netdata service.

</details>


<details>
<summary>Change (Non-Systemd)</summary>

To modify the scheduling policy, [edit](/docs/netdata-agent/configuration/README.md#edit-configuration-files) `netdata.conf`:

```text
[global]
  process scheduling policy = idle
```

**Available Policies**:

| Policy         | Description                                                                                                               |
|----------------|---------------------------------------------------------------------------------------------------------------------------|
| `batch`        | Similar to `other` but treats the thread as CPU-intensive, applying a mild scheduling penalty. This is Netdata's default. |
| `idle`         | Uses CPU only when available (lower than nice 19). Under extreme system load, may cause 1-2 second gaps in charts.        |
| `other`/`nice` | Linux's default process policy. Uses dynamic priorities based on the process's `nice` level.                              |
| `fifo`         | Requires static priorities above 0. Immediately preempts `other`, `batch`, or `idle` threads. No time slicing.            |
| `rr`           | Enhanced `fifo` with maximum time quantum for each thread.                                                                |
| `keep`/`none`  | Maintains existing scheduling policy and priority settings.                                                               |

For additional details about process scheduling, see [man sched](https://man7.org/linux/man-pages/man7/sched.7.html).

**FIFO and RR Priority**:

When using `fifo` or `rr` policies, you can set the process priority in `netdata.conf`:

```text
[global]
    process scheduling priority = 0
```

Priority values range from 0 to 99, with higher values indicating higher process importance.

**Nice Level**

For `other`, `nice`, or `batch` policies, you can adjust the nice level:

```text
[global]
    process nice level = 19
```

The nice level ranges from -20 (the highest priority) to 19 (the lowest priority). A higher value means the process is "nicer" to other processes by using fewer CPU resources.

</details>

## Debugging

When Netdata is compiled with debugging enabled:

- **Performance Impact**: Compiler optimizations are disabled, which may result in slightly reduced performance.
- **Debug Logging**: Disabled by default. To enable logging for specific components:
    - Open `netdata.conf`.
    - Set the `debug flags` option to a hex value that corresponds to the components you want to trace.
    - Debug flag options are defined in [log.h](https://raw.githubusercontent.com/netdata/netdata/master/src/libnetdata/log/log.h) as `D_*` values. Use `0xffffffffffffffff` to enable all debug flags.

> **Important**
>
> Remember to disable debug logging (`debug flags = 0`) after you finish troubleshooting. Debug logs can grow rapidly and consume significant disk space.

### Compiling Netdata with debugging

To compile Netdata with debugging capabilities:

```sh
# Navigate to Netdata source directory
cd /usr/src/netdata.git

# Install with debugging enabled
CFLAGS="-O1 -ggdb -DNETDATA_INTERNAL_CHECKS=1" ./netdata-installer.sh
```

After installation, use the `debug flags` setting in your configuration to specify which components to trace.

This compilation method includes debugging information in the binary and enables internal checks. **This is recommended only for development or troubleshooting purposes**.

### Debugging Crashes

While Netdata is designed to be highly stable, if you encounter a crash, providing stack traces greatly helps in identifying the root cause.

To generate useful debugging information, ensure you have Netdata [compiled with debugging enabled](#debugging).

#### Method 1: Analyzing a Core Dump

If you have a core dump from the crash, run:

```sh
gdb $(which netdata) /path/to/core/dump
```

#### Method 2: Using Valgrind for Reproducible Crashes

If you can reproduce the crash consistently:

1. Install the `valgrind` package
2. Run Netdata under Valgrind:

   ```sh
   valgrind $(which netdata) -D
   ```

Netdata will run significantly slower under Valgrind. When the crash occurs, Valgrind will output the stack trace to your console.

#### Reporting the Issue

For either method:

- Create a [new GitHub issue](https://github.com/netdata/netdata/issues/new/choose).
- Include the complete output from gdb or Valgrind.
- Add any relevant details about the circumstances of the crash.
