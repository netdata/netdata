### Understand the alert

This alert, `load_cpu_number`, calculates the base trigger point for load average alarms, which helps identify when the system is overloaded. The alert checks the maximum number of CPUs in the system over the past 1 minute. If there is only one CPU, the trigger is set at 2.

### What does load average mean?

The term `system load average` on a Linux machine measures the number of threads that are currently working and those waiting to work (CPU, disk, uninterruptible locks). In simpler terms, the load average measures the number of threads that aren't idle.

### What does overloaded mean?

An overloaded system is when the demand on the system's resources (CPUs, disks, etc.) is higher than its capacity to handle tasks. This can lead to increased wait times, slower processing, and in worst cases, system crashes.

### Troubleshoot the alert

1. Determine the current load average on the system:
   
   Use the `uptime` command in the terminal to see the current load average:
   ```
   uptime
   ```

2. Identify if the problem is CPU load or I/O load:

   Use `vmstat` (or `vmstat 1`, to set a delay between updates in seconds) to get a report on system statistics:
   
   The `procs` column shows:
   r: The number of runnable processes (running or waiting for run time).
   b: The number of processes blocked waiting for I/O to complete.

3. Check per-process CPU/disk usage to find the top consumers:

   a. Use `top` to see the processes that are the main CPU consumers:
   ```
   top -o +%CPU -i
   ```

   b. Use `iotop` to monitor Disk I/O usage (install it if not available):
   ```
   sudo iotop
   ```

4. Minimize the load by closing any unnecessary main consumer processes. Double-check if the process you want to close is necessary.

### Useful resources

1. [Unix Load Average Part 1: How It Works](https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works)
2. [Unix Load Average Part 2: Not Your Average Average](https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average)
3. [Understanding Linux Process States](https://access.redhat.com/sites/default/files/attachments/processstates_20120831.pdf)