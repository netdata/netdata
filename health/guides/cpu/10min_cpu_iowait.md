# 10min_cpu_iowait

## OS: Linux

This alarm calculates the average time of `iowait` through 10 minute interval periods. `iowait` is the percentage of
time where there has been at least one I/O request in progress while the CPU has been idle.

We call I/O -on a process level- the use of the read and write services, such as reading data from a physical
drive.

It's important to note that during the time a process waits on I/O, the system can schedule other processes, but
`iowait` is measured specifically while the CPU is idle.

A common example of when this alert might be triggered would be when your CPU requests some data and the device
responsible for it can't deliver it fast enough. As a result the CPU (in the next clock interrupt) is idle, so you
encounter `iowait`. If this persists for some time and the average from the metrics we gather exceeds the value that is
being checked in the `.conf` file, then the alert is raised because the CPU is being bottlenecked by your system’s
disks. <sup> [1](https://blog.pregos.info/wp-content/uploads/2010/09/iowait.txt) [2](
https://serverfault.com/questions/12679/can-anyone-explain-precisely-what-iowait-is) </sup>

<details>
<summary>References and Sources</summary>

[[1] What exactly is "iowait"?](https://blog.pregos.info/wp-content/uploads/2010/09/iowait.txt)  
[[2] Serverfault](https://serverfault.com/questions/12679/can-anyone-explain-precisely-what-iowait-is)
</details>

### Troubleshooting Section

<details>
<summary>Check for main I/O related processes and hardware issues</summary>

- Generally, this issue is caused by having slow hard drives that cannot keep up with the speed of your CPU. You can see
  the percentage of `iowait` by going to your node on Netdata Cloud and clicking the `iowait` dimension under the Total
  CPU Utilization chart.


- You can use `vmstat` (or `vmstat 1`, to set a delay between updates in seconds)

```
root@netdata~ # vmstat 
procs -----------memory---------- ---swap-- -----io---- -system-- ------cpu-----
 r  b   swpd   free   buff  cache   si   so    bi    bo   in   cs us sy id wa st
 8  0 1200384 168456  48840 1461540    4   14    65    51  334  196  3  1 95  0  0
```

The `procs` column, shows;  \
b: The number of processes blocked waiting for I/O to complete.

After that, you can use `ps` and specifically `ps -eo s,user,cmd | grep ^[D]`.

- The `grep` command will fetch the processes that their state code starts with `D` which means uninterruptible sleep
  (usually IO).

> It would be helpful to close any of the main consumer processes, but Netdata strongly suggests knowing exactly what
processes you are closing and being certain that they are not necessary.

- If you see that you don't have a lot of processes that you can terminate (or you need them for your workflow), then
  you would have to upgrade your system’s drives; if you have an HDD, upgrading to an SSD or an NVME drive would make a
  great impact on this metric.

</details>

<details>
<summary>Check your database</summary>

- As another example, in a database environment, you would want to optimize your operations. Check for potential inserts
  on large data sets, keeping in mind that `write` operations take more time than `read`. You should also search for
  complex requests, like large joins and queries over a big data set. These can introduce `iowait` and need to be
  optimized.

</details>
