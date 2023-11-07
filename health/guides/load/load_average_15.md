# load_average_15

## OS: Linux

This alarm calculates the system `load average` (CPU and I/O demand) over the period of fifteen 
minutes.   
If you receive this alarm, it means that your system is "overloaded."

The alert gets raised into warning if the metric is 2 times the expected value and cleared if 
the value is 1.75 times the expected value.

For further information on how our alerts are calculated, please have a look at our [Documentation](
https://learn.netdata.cloud/docs/agent/health/reference#expressions).


<details>
<summary>What does "load average" mean?</summary>

The term `system load average` on a Linux machine, measures the **number of threads that are 
currently working and those waiting to work** (CPU, disk, uninterruptible locks)<sup> [1](https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works) </sup><sup> [2](https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average) </sup>. So simply stated: **System load average measures the number of threads that aren't idle.**

</details>

<details>
<summary>What does "overloaded" mean?</summary>

Andre Lewis explains the term "overloaded" by using an example in his Blog post "Understanding Linux CPU
Load - when should you be worried?"<sup> [3](https://scoutapm.com/blog/understanding-load-averages) </sup>
You can click on the footnote or find it in our links section.

Let's look at a single core CPU system and think of its core count as car lanes on a bridge. A car represents a process in this example:

- On a 0.5 load average, the traffic on the bridge is fine, it is at 50% of its capacity.
- If the load average is at 1, then the bridge is full, and it is utilized 100%.
- If the load average gets to 2 (remember we are on a single core machine), it means that there is one car lane that is passing the bridge. However, there is **another** full car lane that waits to pass the bridge.

So this is how you can imagine CPU load, but keep in mind that `load average` counts also I/O demand, so there is an analogous example there.

</details>

<br>

<details>
<summary>References and Sources</summary>

1. [UNIX Load Average Part 1: How It Works](
   https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works)
2. [UNIX Load Average Part 2: Not Your Average Average](
   https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average)
3. [Understanding Linux CPU Load](https://scoutapm.com/blog/understanding-load-averages)
4. [Linux Load Averages: Solving the Mystery](https://www.brendangregg.com/blog/2017-08-08/linux-load-averages.html)
5. [Understanding Linux Process States](
   https://access.redhat.com/sites/default/files/attachments/processstates_20120831.pdf)
</details>

### Troubleshooting Section

<details>
    <summary>Determine if the problem is CPU or I/O bound</summary>

First you need to check if you are running on a CPU load or an I/O load problem.

1. To get a report about your system statistics, use `vmstat` (or `vmstat 1`, to set a delay between updates in seconds):

```
root@netdata~ # vmstat 
procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu-----
 r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st
 8  0 1200384 168456  48840 1461540    4   14    65    51  334  196  3  1 95  0  0
```

The `procs` column, shows:  
r: The number of runnable processes (running or waiting for run time).  
b: The number of processes blocked waiting for I/O to complete.

2. List your currently running processes using the `ps` command:

The `grep` command will fetch the processes that their state code starts either with R (running or runnable (on run queue)) or D(uninterruptible sleep (usually IO)).

3. Minimize the load by closing any unnecessary main consumer processes. We strongly advise you to double-check if the process you want to close is necessary.

</details>

<details>
    <summary>Check per-process CPU/disk usage to find the top consumers</summary>

1. To see the processes that are the main CPU consumers, use the task manager program `top` like this:

   ```
   root@netdata~ # top -o +%CPU -i
   ```


2. Use `iotop`:  
   `iotop` is a useful tool, similar to `top`, used to monitor Disk I/O usage, if you don't have it,
   then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
   ```
   root@netdata~ # sudo iotop
   ```
Note: If `iotop` is not installed on your machine, please refer to the [install instructions](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)

3. Minimize the load by closing any unnecessary main consumer processes. We strongly advise you to double-check if the process you want to close is necessary.

</details>