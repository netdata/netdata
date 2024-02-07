### Understand the alert

This alarm calculates the system `load average` (`CPU` and `I/O` demand) over the period of one minute. If you receive this alarm, it means that your system is `overloaded`.

### What does "load average" mean?

The term `system load average` on a Linux machine, measures the **number of threads that are currently working and those waiting to work** (CPU, disk, uninterruptible locks). So simply stated: **System load average measures the number of threads that aren't idle.**

### What does "overloaded" mean?

Let's look at a single core CPU system and think of its core count as car lanes on a bridge. A car represents a process in this example:

- On a 0.5 load average, the traffic on the bridge is fine, it is at 50% of its capacity.
- If the load average is at 1, then the bridge is full, and it is utilized 100%.
- If the load average gets to 2 (remember we are on a single core machine), it means that there is one car lane that is passing the bridge. However, there is **another** full car lane that waits to pass the bridge. 

So this is how you can imagine CPU load, but keep in mind that `load average` counts also I/O demand, so there is an analogous example there.

### Troubleshoot the alert

- Determine if the problem is CPU load or I/O load

To get a report about your system statistics, use `vmstat` (or `vmstat 1`, to set a delay between updates in seconds):

The `procs` column, shows:  
r: The number of runnable processes (running or waiting for run time).  
b: The number of processes blocked waiting for I/O to complete.

- Check per-process CPU/disk usage to find the top consumers

1. To see the processes that are the main CPU consumers, use the task manager program `top` like this:

   ```
   top -o +%CPU -i
   ```

2. Use `iotop`:  
   `iotop` is a useful tool, similar to `top`, used to monitor Disk I/O usage, if you don't have it, then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
   ```
   sudo iotop
   ```

3. Minimize the load by closing any unnecessary main consumer processes. We strongly advise you to double-check if the process you want to close is necessary. 

### Useful resources

1. [UNIX Load Average Part 1: How It Works](https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works)  
2. [UNIX Load Average Part 2: Not Your Average Average](https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average)  
3. [Understanding Linux CPU Load](https://scoutapm.com/blog/understanding-load-averages)  
4. [Linux Load Averages: Solving the Mystery](https://www.brendangregg.com/blog/2017-08-08/linux-load-averages.html)  
5. [Understanding Linux Process States](https://access.redhat.com/sites/default/files/attachments/processstates_20120831.pdf)
