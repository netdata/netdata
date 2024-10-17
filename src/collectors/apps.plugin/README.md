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

## Charts

`apps.plugin` offers a set of charts for three groups within the **System->Processes** section of the Netdata dashboard: **Apps**, **Users**, and **Groups**.

Each of these sections presents the same number of charts:

- CPU utilization
    - Total CPU usage
    - User/system CPU usage
- Memory
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

The following methods are used for matching against the specified patterns:

| Method  | Description                                                          |
|---------|----------------------------------------------------------------------|
| comm    | Process name as reported by `ps -e` or `cat /proc/{PID}/comm`        |
| cmdline | The complete command line (`cat /proc/{PID}/cmdline \| tr '\0' ' '`) |

> On Linux, the **comm** field is limited to 15 characters.
> `apps.plugin` attempts to obtain the full process name by searching for it in the **cmdline**.
> If successful, the entire process name is used; otherwise, the shortened version is used.

You can use asterisks (`*`) to create patterns:

| Mode      | Pattern  | Description                              |
|-----------|----------|------------------------------------------|
| prefix    | `name*`  | Matches a **comm** that begins with name |
| suffix    | `*name`  | Matches a **comm** that ends with name   |
| substring | `*name*` | Searches for name within the **cmdline** |

- Asterisks can be placed anywhere within name (e.g., `na*me`) without affecting the matching criteria (**comm** or **cmdline**).
- To include process names with spaces, enclose them in quotes (single or double), like this: `'Plex Media Serv'` or `"my other process"`.
- To include processes with single quotes, enclose them in double quotes: `"process with this ' single quote"`.
- To include processes with double quotes, enclose them in single quotes: `'process with this " double quote'`.
- The order of the entries in the configuration list is crucial. The first matching entry will be used, so it's important to follow a top-down hierarchy. Processes that don't match any entry will inherit the group from their parent processes.

There are a few command line options you can pass to `apps.plugin`. The list of available options can be acquired with the `--help` flag.
The options can be set in the `netdata.conf` using the [`edit-config` script](/docs/netdata-agent/configuration/README.md).

For example, to disable user and user group charts you would set:

```text
[plugin:apps]
  command options = without-users without-groups
```

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
