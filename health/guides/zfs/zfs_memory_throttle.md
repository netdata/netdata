### Understand the alert

This alert indicates the number of times ZFS had to limit the Adaptive Replacement Cache (ARC) growth in the last 10 minutes. ARC stores the most recently used and most frequently used data in RAM, helping to improve read performance. When ARC growth is throttled, it can impact read performance due to a higher chance of cold hits.

### Troubleshoot the alert

1. **Monitor RAM usage**: Check your system's RAM usage to determine if there is sufficient memory available for ARC. If other processes are consuming a large amount of RAM, ARC growth may be throttled to free up resources.

2. **Increase RAM capacity**: If you consistently experience ARC throttling, consider increasing your RAM capacity. This will allow for a larger ARC size, improving read performance and reducing the likelihood of cold hits.

3. **Adjust ARC size**: If you are using ZFS on Linux, you can adjust the ARC size by modifying the `zfs_arc_min` and `zfs_arc_max` parameters in the `/etc/modprobe.d/zfs.conf` file. On FreeBSD, you can adjust the `vfs.zfs.arc_max` sysctl parameter. Make sure to set these values according to your system's RAM capacity and workload requirements.

4. **Evaluate workload**: Analyze your system's workload to identify if there are any specific processes or applications that are causing high memory usage, leading to ARC throttling. Optimize or limit these processes if necessary.


### Useful resources

1. [Linux: ZFS Caching](https://www.45drives.com/community/articles/zfs-caching/)
2. [FreeBSD: OpenZFS documentation](https://openzfs.org/w/index.php?title=Features&mobileaction=toggle_view_mobile#Single_Copy_ARC)
3. [ZFS on Linux Performance Tuning Guide](https://github.com/zfsonlinux/zfs/wiki/Performance-Tuning)
4. [FreeBSD ZFS Tuning Guide](https://wiki.freebsd.org/ZFSTuningGuide)
