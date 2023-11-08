### Understand the alert

This alert presents the average percentage of time the disk was busy over the last 10 minutes. If you receive this it indicates high disk load and that the disk spent most of the time servicing
read or write requests.

This alert is triggered in a warning state when the metric exceeds 98%.

This metric is the same as the %util column on the command `iostat -x`.

### Troubleshoot the alert

- Check per-process disk usage to find the top consumers (If you got this alert for a device serving requests in parallel, you can ignore it)

On Linux use `iotop` to see which processes are the main Disk I/O consumers on the `IO` column.
   ```
   sudo iotop
   ```
  Using this, you can see which processes are the main Disk I/O consumers on the `IO` column.

On FreeBSD use `top`
   ```
   top -m io -o total
   ```
### Useful resources

1. [Two traps in iostat: %util and svctm](https://brooker.co.za/blog/2014/07/04/iostat-pct.html)

2. `iotop` is a useful tool, similar to `top`, used to monitor Disk I/O usage, if you don't have it, then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
