# Function: Top / Processes

## Quick Info

- **Plugin**: `apps.plugin`
- **Type**: Simple Table (real-time snapshot)
- **Availability**: Linux, Windows, FreeBSD, macOS
- **Required Access**: View Agent Config for command line visibility

### System Equivalents

| Operating System | Traditional Tools                                    |
| ---------------- | ---------------------------------------------------- |
| **Linux**        | `top`, `htop`, `ps aux`, `pidstat`                   |
| **Windows**      | Task Manager, `tasklist`, `Get-Process` (PowerShell) |
| **FreeBSD**      | `top`, `ps aux`, `procstat`                          |
| **macOS**        | Activity Monitor, `top`, `ps aux`                    |

The Netdata `processes` function provides several advantages over traditional tools:
- More accurate resource accounting through child process accumulation (e.g., it can accurately provide the CPU utilization of shell scripts)
- Unified cross-platform view with consistent metrics
- Comprehensive I/O and file descriptor metrics per PID
- Direct correlation with Netdata Apps dashboard section

## Purpose

The `processes` function is the drill-down companion to Apps (`apps.plugin`) charts, providing complete visibility into how system resources are broken down by individual processes and how they are aggregated into the categories shown in Netdata dashboards.

`apps.plugin` intelligently groups processes into categories to avoid extreme cardinality issues (millions of potential PIDs). It identifies spawn managers (systemd, containerd, init, etc.) and groups process trees by their top-most parent - the direct children of these spawn managers. This creates a manageable set of categories with accumulated metrics from entire process trees, including exited children.

When users see that "Application X" consumes significant resources in the charts, they need to understand:
- Which specific processes are included in that category
- How resources are distributed among those processes
- Why certain processes are grouped together

The `processes` function answers these questions by showing:
- Every running PID with its assigned `Category` (matching the chart instances)
- Complete resource breakdown per individual process
- Accumulated metrics from exited children (unique capability)

### Key Capabilities
- **Accurate Resource Attribution**: More accurate than top/htop because it includes exited children and normalizes usage to match total system resources
- **Process Tree Understanding**: Shows how processes are grouped into categories via the `Category` field
- **Comprehensive Metrics**: Breaks down CPU (user/system), memory, I/O, file descriptors, threads, and more
- **Leak Detection**: Identify memory leaks, file descriptor leaks, socket leaks, thread leaks
- **Uptime Tracking**: Shows per-process uptime to spot restarts and long-running processes

## Data Fields

| Field               | Type       | Description                                                           | Filterable | Sortable | Groupable | OS Availability        |
| ------------------- | ---------- | --------------------------------------------------------------------- | ---------- | -------- | --------- | ---------------------- |
| **PID**             | Integer    | Process ID                                                            | ✓          | ✓        | ✓         | All                    |
| **Cmd**             | String     | Process command name                                                  | ✓          | ✓        | -         | All                    |
| **Name**            | String     | Process friendly name (if available)                                  | ✓          | ✓        | -         | Windows only           |
| **CmdLine**         | String     | Full command line with arguments (requires elevated access)           | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **PPID**            | Integer    | Parent process ID                                                     | ✓          | ✓        | ✓         | All                    |
| **Category**        | String     | Process category from apps_groups.conf                                | ✓          | ✓        | ✓         | All                    |
| **User**            | String     | User owner of the process                                             | ✓          | ✓        | ✓         | All                    |
| **Uid**             | Integer    | User ID                                                               | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **Group**           | String     | Group owner                                                           | ✓          | ✓        | ✓         | Linux, FreeBSD, macOS  |
| **Gid**             | Integer    | Group ID                                                              | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **CPU**             | Percentage | Total CPU usage (100% = 1 core)                                       | ✓          | ✓        | -         | All                    |
| **UserCPU**         | Percentage | User-space CPU time                                                   | ✓          | ✓        | -         | All                    |
| **SysCPU**          | Percentage | Kernel-space CPU time                                                 | ✓          | ✓        | -         | All                    |
| **GuestCPU**        | Percentage | Guest VM CPU time (if available)                                      | ✓          | ✓        | -         | Linux only             |
| **CUserCPU**        | Percentage | Children user CPU (accumulated from exited children)                  | ✓          | ✓        | -         | Linux, FreeBSD         |
| **CSysCPU**         | Percentage | Children system CPU (accumulated from exited children)                | ✓          | ✓        | -         | Linux, FreeBSD         |
| **CGuestCPU**       | Percentage | Children guest CPU (accumulated from exited children)                 | ✓          | ✓        | -         | Linux only             |
| **vCtxSwitch**      | Rate       | Voluntary context switches per second                                 | ✓          | ✓        | -         | Linux, macOS           |
| **iCtxSwitch**      | Rate       | Involuntary context switches per second                               | ✓          | ✓        | -         | Linux only             |
| **Memory**          | Percentage | Memory usage as percentage of total system RAM                        | ✓          | ✓        | -         | All                    |
| **Resident**        | MiB        | Resident Set Size (physical memory)                                   | ✓          | ✓        | -         | All                    |
| **Estimated**       | MiB        | Estimated memory using PSS scaling (visible by default when enabled)  | ✓          | ✓        | -         | Linux 4.14+ (with PSS) |
| **Pss**             | MiB        | Proportional Set Size (hidden by default)                             | ✓          | ✓        | -         | Linux 4.14+ (with PSS) |
| **PssAge**          | Seconds    | Time since last smaps sample (hidden by default)                      | ✓          | ✓        | -         | Linux 4.14+ (with PSS) |
| **SharedRatio**     | Percentage | Shared memory ratio from PSS (hidden by default)                      | ✓          | ✓        | -         | Linux 4.14+ (with PSS) |
| **Shared**          | MiB        | Shared memory pages                                                   | ✓          | ✓        | -         | Linux only             |
| **Virtual**         | MiB        | Virtual memory size                                                   | ✓          | ✓        | -         | All                    |
| **Swap**            | MiB        | Swap memory usage                                                     | ✓          | ✓        | -         | Linux, Windows         |
| **PReads**          | KiB/s      | Physical disk read rate                                               | ✓          | ✓        | -         | Linux only             |
| **PWrites**         | KiB/s      | Physical disk write rate                                              | ✓          | ✓        | -         | Linux only             |
| **LReads**          | KiB/s      | Logical I/O read rate (includes cache)                                | ✓          | ✓        | -         | All                    |
| **LWrites**         | KiB/s      | Logical I/O write rate (includes cache)                               | ✓          | ✓        | -         | All                    |
| **ROps**            | ops/s      | Read operations per second                                            | ✓          | ✓        | -         | Linux, Windows         |
| **WOps**            | ops/s      | Write operations per second                                           | ✓          | ✓        | -         | Linux, Windows         |
| **MinFlt**          | pgflts/s   | Minor page faults per second                                          | ✓          | ✓        | -         | All                    |
| **MajFlt**          | pgflts/s   | Major page faults per second                                          | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **CMinFlt**         | pgflts/s   | Children minor faults (accumulated)                                   | ✓          | ✓        | -         | Linux, FreeBSD         |
| **CMajFlt**         | pgflts/s   | Children major faults (accumulated)                                   | ✓          | ✓        | -         | Linux, FreeBSD         |
| **FDsLimitPercent** | Percentage | File descriptors usage vs limit                                       | ✓          | ✓        | -         | Linux only             |
| **FDs**             | Count      | Total open file descriptors                                           | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **Files**           | Count      | Open regular files                                                    | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **Pipes**           | Count      | Open pipes                                                            | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **Sockets**         | Count      | Open network sockets                                                  | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **iNotiFDs**        | Count      | iNotify file descriptors                                              | ✓          | ✓        | -         | Linux only             |
| **EventFDs**        | Count      | Event file descriptors                                                | ✓          | ✓        | -         | Linux only             |
| **TimerFDs**        | Count      | Timer file descriptors                                                | ✓          | ✓        | -         | Linux only             |
| **SigFDs**          | Count      | Signal file descriptors                                               | ✓          | ✓        | -         | Linux only             |
| **EvPollFDs**       | Count      | Event poll descriptors                                                | ✓          | ✓        | -         | Linux only             |
| **OtherFDs**        | Count      | Other file descriptors                                                | ✓          | ✓        | -         | Linux, FreeBSD, macOS  |
| **Handles**         | Count      | Open handles (Windows compatibility)                                  | ✓          | ✓        | -         | Windows only           |
| **Processes**       | Count      | Number of processes (1 for single process, >1 for multi-process apps) | ✓          | ✓        | -         | All                    |
| **Threads**         | Count      | Number of threads                                                     | ✓          | ✓        | -         | All                    |
| **Uptime**          | Seconds    | Process uptime                                                        | ✓          | ✓        | -         | All                    |

### Platform-Specific Field Notes

- **Linux**: The most comprehensive data with all metrics including physical I/O, detailed file descriptors, child process accumulation, and resource limits
  - **PSS Memory Estimation** (kernel 4.14+): When enabled (default), provides `Estimated`, `Pss`, `PssAge`, and `SharedRatio` fields for more accurate memory accounting in shared-memory workloads. Disable with `--pss 0` to remove these fields and use traditional RSS measurements.
- **macOS**: Full process data except physical I/O, children accumulation, and some advanced metrics
- **FreeBSD**: Similar to macOS but includes children CPU accumulation
- **Windows**: Different approach using handles instead of file descriptors, includes I/O operations but lacks user/group ownership and command line access

## Drill-Down Workflow

The typical workflow for drilling down to individual processes looks like this:

1. **Observe Chart Anomaly**: Notice high resource usage in an `apps.plugin` chart category (e.g., "web" consuming 80% CPU)
2. **Launch Processes Function**: Open the function to see all processes 
3. **Filter by Category**: Use `category:web` filter to see only processes in that category
4. **Identify Culprit**: Sort by the relevant metric (CPU, Memory, etc.) to find the specific process
5. **Analyze Process Tree**: Use PPID relationships to understand process spawning patterns
6. **Group Analysis**: Group by User, Command, or other fields to understand patterns

## Use Cases

### 1. Break Down System Resources into Processes

The processes function provides complete visibility into how system resources are distributed across all running processes, enabling comprehensive resource accounting and analysis.

#### View resource distribution across all processes
Sort by `CPU`, `Memory`, or `I/O` metrics descending to see which processes consume the most resources. Group by `Category` to understand resource allocation across application groups. This provides a complete breakdown of system resource utilization at the process level.

#### Understand category composition and aggregation
Filter by `category:[name]` to see all processes that contribute to a specific apps.plugin chart instance. Group by `Cmd` within a category to understand which different executables are grouped together. This reveals exactly how Netdata's intelligent grouping works and what's included in each category.

#### Analyze resource usage by user or group
Group processes by `User` or `Group` to understand resource consumption patterns across different users and system accounts. Sort by aggregate CPU or memory within each group to identify which users are consuming the most resources. This helps with multi-tenant resource accounting and fair-share analysis.

### 2. Drill Down to Identify Specific Heavy Consumer Processes

When apps.plugin charts show high resource usage in a category, the processes function enables precise identification of the specific processes responsible.

#### Identify CPU-intensive processes within categories
Filter by `category:[name]` and sort by `CPU` descending to find the exact processes causing high CPU usage in a chart category. Look at both own CPU (`UserCPU`, `SysCPU`) and children CPU (`CUserCPU`, `CSysCPU`) to understand whether the load comes from the process itself or its children.

#### Find memory-consuming processes in application groups
Filter by specific categories and sort by `Resident` or `Memory` percentage to identify which processes within an application group consume the most RAM. Compare `Virtual` vs `Resident` to understand memory allocation patterns and potential over-provisioning.

On Linux 4.14+ with PSS enabled (default), use `Estimated` instead of `Resident` for more accurate memory accounting in shared-memory workloads (databases, cache servers, etc.). The `Estimated` field scales shared memory using PSS ratios to show true proportional memory usage. Check `SharedRatio` to see the scaling factor - values significantly below 100% indicate heavy shared memory usage where `Resident` would overstate consumption. Use `PssAge` to verify data freshness.

#### Locate I/O-heavy processes causing disk bottlenecks
Sort by `PReads + PWrites` for physical I/O or `LReads + LWrites` for logical I/O to find processes generating the most disk activity. Filter by category to drill down from chart-level I/O metrics to specific process-level I/O patterns.

### 3. Detect Leaks of Multiple Kinds

The processes function excels at identifying various types of resource leaks by correlating resource usage with process uptime.

#### Memory leak detection in long-running processes
Filter processes with `Uptime > 3600` (one hour) and sort by `Resident` (or `Estimated` on Linux with PSS enabled) memory descending. Look for processes where memory consumption is disproportionately high relative to their uptime. Track specific PIDs over time to observe continuously growing memory usage patterns. On shared-memory workloads, use `Estimated` to avoid false positives from shared pages that aren't actually leaking.

#### File descriptor leak identification
Sort by `FDs` count or filter for `FDsLimitPercent > 50` to find processes approaching their file descriptor limits. Examine the breakdown of descriptor types (`Files`, `Sockets`, `Pipes`, etc.) to understand what type of resources are leaking. Correlate high FD counts with process uptime to identify gradual leaks.

#### Socket and network connection leaks
Sort by `Sockets` count to identify processes with abnormally high network connections. Compare socket counts against expected application behavior and uptime to detect connection leaks. Group by `Category` to see if entire application groups are affected by socket exhaustion.

#### Thread leak monitoring
Sort by `Threads` count and correlate with `Uptime` to find processes creating threads without proper cleanup. Look for processes where thread count grows continuously over time. Filter by category to identify applications with thread pool management issues.

### 4. Monitor Crashes or Abnormal Events via Uptime

Process uptime tracking enables detection of crashes, restarts, and abnormal process lifecycle events.

#### Detect recent process restarts and crashes
Sort by `Uptime` ascending to immediately see which processes have recently started or restarted. Filter by specific categories or command names to monitor critical services for unexpected restarts. Compare process start times with known maintenance windows to identify unplanned restarts.

#### Identify unstable applications with frequent restarts
Group processes by `Cmd` and look for multiple PIDs with similar names but different uptimes, indicating repeated restarts. Track specific application categories over time to identify patterns of instability. Correlate low uptimes with high child CPU accumulation to detect crash loops.

#### Monitor process lifecycle and stability patterns
Filter by category and examine uptime distribution to understand application stability. Look for processes that should be long-running but have short uptimes. Use PPID relationships to identify parent processes that frequently spawn short-lived children.

### 5. Security Monitoring

The processes function provides critical security visibility by exposing process ownership, privileges, and behavior patterns.

#### Detect unauthorized or suspicious processes
Filter by `Category:other` to find uncategorized processes that may be suspicious. Sort by `User` to identify processes running under unexpected accounts. Search for unusual command names or paths that don't match normal system behavior.

#### Monitor privilege escalation and root processes
Filter by `Uid:0` or `User:root` to track all processes running with root privileges. Group root processes by `Cmd` to understand what's running with elevated permissions. Look for unexpected processes running as root that shouldn't require privileges.

#### Analyze network activity and connection patterns
Sort by `Sockets` count to identify processes with unusual network activity. Filter by specific users or categories to detect abnormal network behavior patterns. Correlate high socket counts with process names to identify potential backdoors or data exfiltration.

#### Track command line arguments for security forensics
Use full-text search in `CmdLine` to find processes launched with specific parameters or scripts. Group by command line patterns to identify potentially malicious execution patterns. Filter by user and examine command lines to detect privilege abuse or policy violations.

## Special Features

- **Child Process Accumulation**: Uniquely captures resources from exited children - critical for accurate measurement of shell scripts and applications that spawn many short-lived processes (even 100+ commands/second)
- **PSS Memory Estimation** (Linux 4.14+): Provides accurate memory accounting for shared-memory workloads by using Proportional Set Size (PSS) to scale shared pages. Enabled by default with periodic sampling to minimize overhead. Shows true memory consumption vs inflated RSS values for databases and cache servers.
- **Category Correlation**: The `Category` field directly matches the instance names in `apps.plugin` charts, enabling drill-down from chart to process level
- **Intelligent Grouping**: Understands spawn managers (systemd, containerd, init) and groups by top-most parent to create manageable categories
- **Normalized Metrics**: All per-process usage is normalized to accurately match total system resource usage
- **Real-time Updates**: Data refreshes every few seconds showing current process state
- **Custom Grouping**: `apps_groups.conf` allows defining custom spawn managers and individual processes of interest
- **Comprehensive FD Breakdown**: Detailed categorization of all file descriptor types for leak detection

## Performance Considerations

- Function executes with minimal overhead using efficient process enumeration
- Large process counts (>1000) may increase response time
- Command line access requires additional security permissions
- No historical data - shows current snapshot only

## Requirements and Limitations

- **Operating System**: Linux, Windows, FreeBSD, macOS
- **Permissions**: Standard user can see basic data; elevated access needed for command lines
- **Data Type**: Real-time snapshot (no historical data)
- **Child Processes**: Only terminated children are accumulated; running children appear separately
- **Platform Variations**: Some fields may not be available on all operating systems (e.g., certain I/O metrics on macOS)

## Related Functions

- `systemd-services`: Aggregated view of processes grouped by systemd service
- `containers-vms`: Container and VM-specific process information
- `network-connections`: Network connections per process
- `systemd-journal`: Process logs and events