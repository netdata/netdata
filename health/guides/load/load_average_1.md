# load_average_1

## OS: Linux

This alarm calculates the system `load average` (`CPU` and `I/O` demand) over the period of one minute.   
If you receive this alarm, it means that your system is `overloaded`.

<details>
<summary>What does "load average" mean</summary>

The term `system load average` on a Linux machine, measures the **number of threads that are currently working and those
waiting to work** (CPU, disk, uninterruptible locks)
<sup> [1](https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works) </sup>
<sup> [2](https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average) </sup>
. So simply stated: **it measures the number of threads that aren't idle.**

</details>

<details>
<summary>What does "overloaded" mean</summary>

The term `overloaded` can be better illustrated using an example as ***Andre Lewis*** says in ***Understanding Linux CPU
Load - when should you be worried?***<sup> [3](https://scoutapm.com/blog/understanding-load-averages) </sup>, which you
find in our links section.

We are going to take a single core CPU system and think of its core count as bridge lanes.

- On a 0.5 load average, the traffic on the bridge is fine, it is at 50% of its capacity.


- If the load average is at 1, then the bridge is full, and it is utilized 100%.


- If the load average gets to 2 *(remember we are on a single core machine)*, it means that there is one lane that is
  passing the bridge and one **other** full lane that waits on the side. On this example, traffic and thus cars, are
  processes.

So this is how you can imagine CPU load, but keep in mind that `load average` counts also I/O demand, so there is an
analogous example there.

</details>

<details>
<summary>How we calculate the alarm</summary>

On Netdata, in the [load.conf](https://github.com/netdata/netdata/blob/master/health/health.d/load.conf) file, under the
health.d directory, you can see how we calculate *when* the alarm should be raised.

- First, there is `load_cpu_number` where it provides the `load_average` alarms with the core count of the machine.


- In the line `warn: ($this * 100 / $load_cpu_number) > (($status >= $WARNING) ? 700 : 800)`,  \
  `($this * 100 / $load_cpu_number)` is the current system load average in %.


- Last, we check if that value exceeds 700% or 800% (depending on the `$status` of the alarm).

</details>

<br>

<details>
<summary>References</summary>

[[1] UNIX Load Average Part 1: How It Works](
https://www.helpsystems.com/resources/guides/unix-load-average-part-1-how-it-works)  \
[[2] UNIX Load Average Part 2: Not Your Average Average](
https://www.helpsystems.com/resources/guides/unix-load-average-part-2-not-your-average-average)  \
[[3] Understanding Linux CPU Load](https://scoutapm.com/blog/understanding-load-averages)  \
[Linux Load Averages: Solving the Mystery](https://www.brendangregg.com/blog/2017-08-08/linux-load-averages.html)  \
[Understanding Linux Process States](
https://access.redhat.com/sites/default/files/attachments/processstates_20120831.pdf)
</details>

### Troubleshooting Section

<details>
    <summary>Determine if the problem is CPU or I/O bound</summary>

First you need to check if you are running on a CPU load or an I/O load problem.

- You can use `vmstat` (or `vmstat 1`, to set a delay between updates in seconds)

```
root@netdata~ # vmstat 
procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu-----
 r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st
 8  0 1200384 168456  48840 1461540    4   14    65    51  334  196  3  1 95  0  0
```

The `procs` column, shows;  \
r: The number of runnable processes (running or waiting for run time).  \
b: The number of processes blocked waiting for I/O to complete.

After that, you can use the `ps` and specifically `ps -eo s,user,cmd | grep ^[RD]`.

- The `grep` command will fetch the processes that their state code starts either with R (running or runnable (on run
  queue)) or D(uninterruptible sleep (usually IO)).

It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what
processes you are closing and being certain that they are not necessary.

</details>

<details>
    <summary>Check per-process CPU/disk usage to find the top consumers</summary>

1. Use `top`:

   ```
   root@netdata~ # top -o +%CPU -i
   ```
   Here, you can see which processes are the main cpu consumers on the `%CPU` column.


2. Use `iotop`:  \
   `iotop` is a useful tool, similar to `top`, used to monitor Disk I/O usage, if you don't have it,
   then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
   ```
   root@netdata~ # sudo iotop
   ```
   Using this, you can see which processes are the main Disk I/O consumers on the `IO` column.

It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what
processes you are closing and being certain that they are not necessary.

</details>
