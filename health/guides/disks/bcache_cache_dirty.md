### Understand the Alert

`bcache` is a cache in the block layer of the Linux Kernel. **It allows fast storage devices**, as SSDs (Solid State Drives), **to act as a cache for slower storage devices**, such as HDDs (Hard Disk Drives). As a result, **hybrid volumes are made with performance improvements**. Generally, a cache device is divided up into `buckets`, matching the physical disk's erase blocks.

This alert indicates that your SSD cache is too small, and overpopulated with data.

You can view `bcache_cache_dirty` as the `bcache` analogous metric to `dirty memory`. `dirty memory` is memory that has been changed but has not yet been written out to disk. For example, you make a change to a file but do not save it. These temporary changes are stored in memory, waiting to be written to disk. So `dirty` data on `bcache` is data that is stored on the cache disk and waits to be written to the backing device (Normally your HDD).

`dirty` data is data in the cache that has not been written to the backing device (normally your HDD). So when the system shuts down, the cache device and the backing device are not safe to be separated. 
`metadata` in general, is data that provides information about other data.

### Troubleshoot the Alert

- Upgrade your cache's capacity

This alert is raised when there is more than 70% *(for warning status)* of your cache populated by `dirty` data and `metadata`, it means that your current cache device doesn't have the capacity to support your workflow. Using a bigger
capacity device as cache can solve the problem.

- Monitor cache usage regularly

Keep an eye on the cache usage regularly to understand the pattern of how your cache gets filled up with dirty data and metadata. This can help you better manage the cache and take proactive measures before facing a performance bottleneck.

   To monitor cache usage, use `cat` command on the cache device's sysfs directory like this:
   
   ```
   cat /sys/fs/bcache/<CACHE_DEV_UUID>/cache0/bcache/stats_five_minute/cache_hit_ratio
   ```
   
   Replace `<CACHE_DEV_UUID>` with your cache device's UUID.

- Periodically write dirty data to the backing device

If the cache becomes frequently filled with dirty data, you can try periodically writing dirty data to the backing device to create more space in the cache. This can especially help if your caching device isn't frequently reaching its full capacity.

   To perform this, you can use the `cron` job scheduler to run a command that flushes dirty data to the HDD periodically. Add the following line to your crontab:

   ```
   */5 * * * * echo writeback > /sys/fs/bcache/<CACHE_DEV_UUID>/cache0/bcache/writeback_rate_debug
   ```

   Replace `<CACHE_DEV_UUID>` with your cache device's UUID. This configuration will flush the dirty data to the backing device every 5 minutes.

- Check for I/O bottlenecks

If you experience performance issues with bcache, it's essential to identify the cause, which could be I/O bottlenecks. Look for any I/O errors or an overloaded I/O subsystem that may be affecting your cache device's performance.

   To check I/O statistics, you can use tools like `iotop`, `iostat` or `vmstat`:

   ```bash
   iotop
   iostat -x -d -z -t 5 5 # run 5 times with a 5-second interval between each report
   vmstat -d
   ```
   
   Analyze the output and look for any signs of a bottleneck, such as excessive disk utilization, slow transfer speeds, or high I/O wait times.

- Optimize cache configuration

Review your current cache configuration and make sure it's optimized for your system's workload. In some cases, adjusting cache settings could help improve the hit ratio and reduce the amount of dirty data.

   To view the bcache settings:

   ```
   cat /sys/fs/bcache/<CACHE_DEV_UUID>/cache0/bcache/*
   ```

   Replace `<CACHE_DEV_UUID>` with your cache device's UUID.

   You can also make changes to the cache settings by echoing the new values to the corresponding sysfs files. Please refer to the [Cache Settings section in the Bcache documentation](https://www.kernel.org/doc/Documentation/bcache.txt) for more details.

### Useful resources

1. [Bcache documentation](https://www.kernel.org/doc/Documentation/bcache.txt)
2. [Arch Linux Wiki: Bcache](https://wiki.archlinux.org/title/bcache)
