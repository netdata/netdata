### Understand the alert

The `zfs_pool_state_crit` alert indicates that your ZFS pool is faulted or unavailable, which can cause access and data loss problems. It is important to identify the current state of the pool and take corrective actions to remedy the situation.

### Troubleshoot the alert

1. **Check the current ZFS pool state**

   Run the `zpool status` command to view the status of all ZFS pools:
   
   ```
   zpool status
   ```
   
   This will display the pool state, device states, and any errors that occurred. Take note of any devices that are in DEGRADED, FAULTED, UNAVAIL, or OFFLINE states.

2. **Assess the problematic devices**

   Check for any hardware issues or file system errors on the affected devices. For example, if a device is FAULTED due to a hardware failure, replace the device. If a device is UNAVAIL or OFFLINE, check the connectivity and make sure it's properly accessible.

3. **Repair the pool**

   Depending on the root cause of the problem, you may need to take different actions:

   - Repair file system errors using the `zpool scrub` command. This will initiate a scrub, which attempts to fix any errors in the pool.
   
      ```
      zpool scrub [pool_name]
      ```

   - Replace a failed device using the `zpool replace` command. For example, if you have a new device `/dev/sdb` that will replace `/dev/sda`, run the following command:

      ```
      zpool replace [pool_name] /dev/sda /dev/sdb
      ```

   - Bring an OFFLINE device back ONLINE using the `zpool online` command:

      ```
      zpool online [pool_name] [device]
      ```

   Note: Make sure to replace `[pool_name]` and `[device]` with the appropriate values for your system.

4. **Verify the pool state**

   After taking the necessary corrective actions, run the `zpool status` command again to verify that the pool state has improved.

5. **Monitor pool health**

   Continuously monitor the health of your ZFS pools to avoid future issues. Consider setting up periodic scrubs and reviewing system logs to catch any hardware or file system errors.

### Useful resources

1. [Determining the Health Status of ZFS Storage Pools](https://docs.oracle.com/cd/E19253-01/819-5461/gamno/index.html)
2. [Chapter 11, Oracle Solaris ZFS Troubleshooting and Pool Recovery](https://docs.oracle.com/cd/E53394_01/html/E54801/gavwg.html)
3. [ZFS on FreeBSD documentation](https://docs.freebsd.org/en/books/handbook/zfs/)
4. [OpenZFS documentation](https://openzfs.github.io/openzfs-docs/)