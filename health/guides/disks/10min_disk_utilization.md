# 10min_disk_utilization

## OS: Linux, FreeBSD

This alert presents the average percentage of time the disk was busy over the
last 10 minutes.  
If you receive this it indicates high disk load and that the disk spent most of the time servicing
read or write requests.

- This alert is triggered in a warning state when the metric exceeds 98%.

This metric is the same as the %util column on the command `iostat -x`:

> %util is the percentage of the time the drive was doing at least one thing.  
> Device saturation occurs when this value is close to 100% for devices serving requests serially.
> But for devices serving requests in parallel, such as RAID arrays and modern SSDs, this number
> does not reflect their performance limits.  
> As a measure of general IO busyness %util is fairly handy, but as an indication of how much the
> system is doing compared to what it can do, it's terrible.<sup>[1](
> https://brooker.co.za/blog/2014/07/04/iostat-pct.html) </sup>



<details>
<summary>References and Sources</summary>

1. [Two traps in iostat: %util and svctm](https://brooker.co.za/blog/2014/07/04/iostat-pct.html)

</details>

### Troubleshooting Section

#### Check per-process disk usage to find the top consumers

> Note: If you got this alert for a device serving requests in parallel, you can ignore it.

<details><summary>Use `iotop` on Linux</summary>

  `iotop` is a useful tool, similar to `top`, used to monitor Disk I/O usage, if you don't have it,
  then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
   ```
   root@netdata~ # sudo iotop
   ```
  Using this, you can see which processes are the main Disk I/O consumers on the `IO` column.

</details>

<details><summary>Use `top` on FreeBSD</summary>

You can use `top`:
   ```
   root@netdata~ # top -m io -o total
   ```
  The `-m io` sets `top` to display I/O statistics, and the `-o total` indicates the results will be
  ordered according to the field "Total".

</details>
