# Applications monitoring (apps.plugin)

`apps.plugin` monitors the resources utilization of all processes running.

## Process Aggregation and Grouping

`apps.plugin` aggregates processes in three distinct ways to provide a more insightful breakdown of resource utilization:

| Grouping   | Description                                                                                                                                        |
|------------|----------------------------------------------------------------------------------------------------------------------------------------------------|
| App        | Grouped by the position in the process tree. This is customizable and allows aggregation by process managers and individual processes of interest. |
| User       | Grouped by the effective user (UID) under which the processes run.                                                                                 |
| User Group | Grouped by the effective group (GID) under which the processes run.                                                                                |

## Short-Lived Process Handling

`apps.plugin` accurately captures resource utilization for both running and exited processes, ensuring that the impact of short-lived subprocesses is fully accounted for.
This is particularly valuable for scenarios where processes spawn numerous short-lived subprocesses, such as shell scripts that fork hundreds or thousands of times per second.
Even though these subprocesses may have a brief lifespan, `apps.plugin` effectively aggregates their resource utilization, providing a comprehensive overview of how resources are shared among all processes within the system.

## PSS Memory Estimation

On Linux systems with kernel 4.14 or later, `apps.plugin` uses Proportional Set Size (PSS) data to provide more accurate memory usage estimates for processes that use shared memory.

PSS is an expensive kernel operation that requires scanning all shared memory segments of a process to determine which memory pages are shared with other processes, and then proportionally dividing the shared memory among them to calculate each process's actual memory footprint. Since PSS for any process can change due to actions by other processes (such as mapping or unmapping the same files, or processes exiting), maintaining accurate real-time PSS data for all processes would be prohibitively expensive.

To balance accuracy with performance, `apps.plugin` uses a **ratio-based estimation approach**: it periodically samples PSS values to calculate a PSS/RSS ratio for each process, then applies this cached ratio to the current RSS values **every second** to estimate memory usage. This means that estimated memory values are updated every second based on current RSS, while the ratio itself is recalibrated adaptively based on process priority.

The plugin implements an **adaptive sampling strategy** designed to prioritize the largest memory consumers and processes with significant memory changes, refreshing them within seconds of detection, while guaranteeing that all processes are eventually sampled within **twice the configured interval** (10 minutes by default for a 5-minute interval). The plugin alternates between two complementary prioritization strategies each iteration:

1. **Delta-Based Strategy**: Prioritizes processes with the largest changes in shared memory, ensuring rapid detection and response to memory growth. Large memory consumers (databases, cache servers, etc.) are typically refreshed within seconds when their memory footprint changes significantly.

2. **Age-Based Strategy**: Prioritizes processes that haven't been sampled longest, ensuring eventual consistency for all processes. Even the smallest processes are guaranteed to be refreshed within twice the configured interval.

Both strategies sort candidates by priority and refresh the top N processes within the configured budget each iteration. By alternating between these strategies, the plugin ensures responsive tracking of significant memory changes while maintaining bounded staleness for all processes.

Additionally, on the first iteration after startup, the plugin samples all processes to establish accurate initial estimates before switching to the adaptive sampling strategy.

## Charts

`apps.plugin` offers a set of charts for three groups within the **System->Processes** section of the Netdata dashboard: **Apps**, **Users**, and **Groups**.

Each of these sections presents the same number of charts:

- CPU utilization
    - Total CPU usage
    - User/system CPU usage
- Memory
    - Estimated Memory Usage (RSS with PSS scaling, default on Linux 4.14+)
    - Memory RSS Usage
    - Real Memory Used (non-shared)
    - Virtual Memory Allocated
    - Minor page faults (i.e. memory activity)
- Swap memory
    - Swap memory used
    - Major page faults (i.e. swap activity)
- Disk
    - Physical reads/writes
    - Logical reads/writes
- Tasks
    - Threads
    - Processes
- FDs
    - Open file descriptors limit %
    - Open file descriptors
- Uptime
    - Carried over uptime (since the last Netdata Agent restart)

In addition, if the [eBPF collector](/src/collectors/ebpf.plugin/README.md) is running, your dashboard will also show an
additional [list of charts](/src/collectors/ebpf.plugin/README.md#integration-with-appsplugin) using low-level Linux
metrics.

## Performance

`apps.plugin` is designed to be highly efficient, collecting significantly more process information than other similar tools while maintaining exceptional speed.
However, due to its comprehensive approach of traversing the entire process tree on each iteration, its resource usage may become noticeable, especially on systems with a large number of processes.

Under Linux, `apps.plugin` reads multiple `/proc` files for each running process, performing this operation on a per-second basis.
This can lead to increased CPU consumption on hosts with several thousands of processes.

In such cases, you may need to adjust the data collection frequency to reduce the plugin's resource usage.

To do this, edit `/etc/netdata/netdata.conf` and find this section:

```text
[plugin:apps]
 # update every = 1
 # command options =
```

Uncomment the `update every` line and set it to a higher value.
For example, setting it to 2 will halve the plugin's CPU usage and collect data once every 2 seconds.

## Configuration

The configuration file is `/etc/netdata/apps_groups.conf`. You can edit this
file using our [`edit-config`](/docs/netdata-agent/configuration/README.md#edit-a-configuration-file-using-edit-config) script.

### Configuring process managers

`apps.plugin` needs to know the common process managers, which are the processes that spawn other processes.
These process managers allow `apps.plugin` to automatically include their subprocesses in the monitoring process, ensuring that important processes are not overlooked.

- Process managers are configured in the `apps_groups.conf` file using the `managers:` prefix, as follows:

  ```text
  managers: process1 process2 process3
  ```

- Multiple lines can be used to define additional process managers, all starting with `managers:`.

- If you want to clear all existing process managers, you can use the line `managers: clear`. This will remove all previously configured managers, allowing you to provide a new list.

### Configuring interpreters

Interpreted languages like `python`, `bash`, `sh`, `node`, and others may obfuscate the actual name of a process.

To address this, `apps.plugin` allows you to configure interpreters and specify that the actual process name can be found in one of the command-line parameters of the interpreter.
When a process matches a configured interpreter, `apps.plugin` will examine all the parameters of the interpreter and locate the first parameter that is an absolute filename existing on disk. If such a filename is found, `apps.plugin` will name the process using the name of that filename.

- Interpreters are configured in the `apps_groups.conf` file using the `interpreters:` prefix, as follows:

```text
interpreters: process1 process2 process3
```

- Multiple lines can be used to define additional process managers, all starting with `interpreters:`.

- If you want to clear all existing process interpreters, you can use the line `interpreters: clear`. This will remove all previously configured interpreters, allowing you to provide a new list.

### Configuring process groups and renaming processes

- The configuration file supports multiple lines, each following this format:

  ```text
  group: process1 process2 ...
  ```

- You can define a group multiple times to include additional processes within it.

- For each process specified, all of its subprocesses will be automatically grouped, not just the matched process itself.

### Matching processes

`apps.plugin` uses different fields for process matching depending on the operating system:

#### Unix-like systems (Linux, FreeBSD, macOS)

| Field   | Description                      | Example                                 |
|---------|----------------------------------|-----------------------------------------|
| comm    | Process name (command)           | `chrome`                                |
| cmdline | Full command line with arguments | `/usr/bin/chrome --enable-features=...` |

> **Note:** On Linux specifically, the **comm** field is limited to 15 characters from `/proc/{PID}/comm`.
> `apps.plugin` attempts to obtain the full process name by searching for it in the **cmdline**.
> If successful, the entire process name is used; otherwise, the shortened version is used.

#### Windows process fields

| Field   | Description                                                      | Example                                                 |
|---------|------------------------------------------------------------------|---------------------------------------------------------|
| comm    | Performance Monitor instance name (may include instance numbers) | `chrome#12`                                             |
| cmdline | Full path to the executable (without command line arguments)     | `C:\Program Files\Google\Chrome\Application\chrome.exe` |
| name    | Friendly name from file description or service display name      | `Google Chrome`                                         |

> On Windows:
> - All pattern types (exact, prefix, suffix, substring) also match against the **name** field
> - The **name** field is preferred for default grouping when no pattern matches
> - Instance numbers (e.g., `#1`, `#2`) are automatically stripped from the **comm** field
> - The `.exe` extension is automatically removed for cleaner display
> - For services (especially `svchost.exe`), the service display name is resolved

#### Pattern matching

You can use asterisks (`*`) to create patterns:

> **Version differences:**
> - **Netdata v2.5.2 and earlier**: Pattern matching is case sensitive
> - **Netdata v2.5.3 and later**: Pattern matching is case insensitive
> - **Netdata v2.5.2 and earlier**: Windows patterns match against `comm` and `cmdline` fields
> - **Netdata v2.5.3 and later**: Windows patterns match against `comm`, `cmdline`, and `name` (friendly name) fields

| Mode      | Pattern     | Description                            | Unix-like                 | Windows           |
|-----------|-------------|----------------------------------------|---------------------------|-------------------|
| exact     | `firefox`   | Matches **comm** exactly               | ✓ Yes                     | ✓ Yes             |
| prefix    | `firefox*`  | Matches **comm** starting with firefox | ✓ Yes                     | ✓ Yes             |
| suffix    | `*fox`      | Matches **comm** ending with fox       | ✓ Yes                     | ✓ Yes             |
| substring | `*firefox*` | Searches within **cmdline**            | ✓ Yes (full command line) | ✓ Yes (full path) |

**Note on substring matching (`*pattern*`):**

- On Unix-like systems: Searches within the full command line including arguments
- On Windows: Searches within the full executable path (e.g., `C:\Program Files\Mozilla Firefox\firefox.exe`)

- Asterisks can be placed anywhere within pattern (e.g., `fi*fox`) without affecting the matching criteria (**comm** or **cmdline**).
- To include process names with spaces, enclose them in quotes (single or double), like this: `'Plex Media Serv'` or `"my other process"`.
- To include processes with single quotes, enclose them in double quotes: `"process with this ' single quote"`.
- To include processes with double quotes, enclose them in single quotes: `'process with this " double quote'`.
- The order of the entries in the configuration list is crucial. The first matching entry will be used, so it's important to follow a top-down hierarchy. Processes that don't match any entry will inherit the group from their parent processes.

#### Windows default grouping behavior

On Windows, when a process doesn't match any pattern in `apps_groups.conf`:

- The **name** field (friendly name from file description or service display name) is used as the default group/category if available
- If no **name** field exists, the **comm** field is used
- This provides better default grouping for Windows services and applications with descriptive names

For example, a process might have:

- **comm**: `svchost`
- **name**: `Windows Update`
- **Default category**: `Windows Update` (uses the friendly name)

### Windows path handling

When configuring `apps_groups.conf` on Windows systems:

1. **No backslash escaping needed** - Windows paths with backslashes are handled as literal strings:
   ```text
   sqlserver: "C:\Program Files\Microsoft SQL Server\MSSQL15.MSSQLSERVER\MSSQL\Binn\sqlservr.exe"
   ```

2. **Use quotes for paths with spaces**:
   ```text
   office: "Microsoft Word" "Microsoft Excel"
   browsers: chrome firefox msedge
   ```

3. **Prefer process names over full paths** - This is more portable and easier to maintain:
   ```text
   # Recommended - matches all SQL Server processes regardless of version/instance
   sqlserver: sqlservr
   
   # Also works but less flexible
   sqlserver: "C:\Program Files\Microsoft SQL Server\MSSQL15.MSSQLSERVER\MSSQL\Binn\sqlservr.exe"
   ```

4. **Use wildcards for flexible path matching**:
   ```text
   # Match anything from Program Files
   programfiles: "*Program Files*"
   
   # Match SQL Server components across versions
   mssql: "*\Microsoft SQL Server\*"
   
   # Match enterprise backup solutions
   backup: "*\Veeam\*" "*\Veritas\*" "*\CommVault\*"
   ```

### Verifying your configuration

You can use the Netdata `processes` function to verify that your `apps_groups.conf` configuration is working correctly:

1. **Access the processes function** through Netdata Cloud (required for security reasons)
2. **Review the output** to see:
    - Current running processes with their `comm`, `cmdline`, and (on Windows) `name` fields
    - The **Category** column shows which group from `apps_groups.conf` each process has been assigned to
    - Resource utilization for each process

3. **Troubleshooting tips**:
    - If a process shows the wrong Category, check the exact process name in the function output
    - On Windows, remember that the `name` field is used for default categories but NOT for pattern matching
    - Remember that the first matching pattern wins - check your pattern order
    - For inherited groups, verify the parent process has the correct Category

There are a few command line options you can pass to `apps.plugin`. The list of available options can be acquired with the `--help` flag.
The options can be set in the `netdata.conf` using the [`edit-config` script](/docs/netdata-agent/configuration/README.md).

For example, to disable user and user group charts you would set:

```text
[plugin:apps]
  command options = without-users without-groups
```

### Memory Estimation with PSS Sampling

On Linux systems with kernel 4.14 or later, `apps.plugin` uses Proportional Set Size (PSS) data from `/proc/<pid>/smaps_rollup` to provide more accurate memory usage estimates for processes that heavily use shared memory (e.g., databases, shared memory applications).

**By default, PSS sampling is disabled**. When disabled, memory charts show traditional RSS (Resident Set Size), which may overstate usage for processes sharing memory pages. Enabling PSS sampling allows the plugin to periodically sample PSS values and use them to scale the shared portion of RSS, providing a significantly more accurate estimate without the overhead of reading smaps on every iteration.

#### Configuration

The `--pss` option controls PSS sampling behavior:

```text
[plugin:apps]
  command options = --pss 5m
```

**Valid values:**

- Duration (e.g., `5m`, `300s`, `10m`): Sets the refresh interval for PSS sampling. Lower values provide more accurate estimates but increase CPU overhead.
- `off` or `0`: Completely disables PSS sampling. Memory charts will show traditional RSS-based measurements.

**Default:** `off`

**How it works:**

- `apps.plugin` uses adaptive sampling that alternates between two strategies each iteration:
    - **Delta-based**: Prioritizes processes with largest shared memory changes (refreshes big memory consumers within seconds)
    - **Age-based**: Prioritizes processes with oldest samples (ensures all processes refreshed within 2× the interval)
- The sampled PSS/RSS ratio is cached and applied to subsequent RSS readings to estimate current memory usage
- This approach ensures rapid response to significant memory changes while guaranteeing bounded staleness for all processes
- When disabled (`--pss 0` or `--pss off`), no PSS sampling occurs and estimated memory charts are not shown

**Performance considerations:**

- Reading `/proc/<pid>/smaps_rollup` is more expensive than reading `/proc/<pid>/status`
- Shorter refresh periods provide more accurate estimates but increase CPU usage
- On systems with thousands of processes, consider increasing the refresh period (e.g., `10m` or `15m`)
- For systems without significant shared memory usage, disabling PSS sampling (`--pss off`) reduces overhead

**Chart behavior:**

- **When PSS is enabled:** Shows both "Estimated memory usage (RSS with shared scaling)" and "Memory RSS usage" charts
- **Default (PSS disabled):** Shows only "Memory RSS usage" charts
- The `processes` function API exposes additional columns (PSS, PssAge, SharedRatio) when PSS is enabled

### Integration with eBPF

If you don't see charts under the **eBPF syscall** or **eBPF net** sections, you should edit your
[`ebpf.d.conf`](/src/collectors/ebpf.plugin/README.md#configure-the-ebpf-collector) file to ensure the eBPF program is enabled.

Also see our [guide on troubleshooting apps with eBPF metrics](/docs/developer-and-contributor-corner/monitor-debug-applications-ebpf.md) for ideas on how to interpret these charts in a few scenarios.

## Permissions

`apps.plugin` requires additional privileges to collect all the necessary information.

During Netdata installation, `apps.plugin` is granted the `cap_dac_read_search` and `cap_sys_ptrace+ep` capabilities.
If this fails (i.e., `setcap` fails), `apps.plugin` is setuid to `root`.

## Security

`apps.plugin` operates on a one-way communication model, sending metrics to Netdata without receiving instructions. This design minimizes potential security risks.

Although `apps.plugin` can function without escalated privileges, it may not be able to collect all the necessary information. To ensure comprehensive data collection, it's recommended to grant the required privileges.

The increased privileges are primarily used for building the process tree in memory, iterating over running processes, collecting metrics, and sending them to Netdata. This process does not involve any external communication or user interaction, further reducing security concerns.
