### Understand the alert

This alert, `mdstat_nonredundant_last_collected`, is triggered when the Netdata Agent fails to collect data from the Multiple Device (md) driver for a certain period. The md driver is used to manage software RAID arrays in Linux.

### What is the md driver?

The md (multiple device) driver is responsible for managing software RAID arrays on Linux systems. It provides a way to combine multiple physical disks into a single logical disk, increasing capacity and providing redundancy, depending on the RAID level. Monitoring the status of these devices is crucial to ensure data integrity and redundancy.

### Troubleshoot the alert

1. Check the status of the md driver:

   To inspect the status of the RAID arrays managed by the md driver, use the `cat` command:

   ```
   cat /proc/mdstat
   ```

   This will display the status and configuration of all active RAID arrays. Look for any abnormal status, such as failed or degraded disks, and replace or fix them as needed.

2. Verify the Netdata configuration:

   Ensure that the Netdata Agent is properly configured to collect data from the md driver. Open the `netdata.conf` configuration file found in `/etc/netdata/` or `/opt/netdata/etc/netdata/`, and look for the `[plugin:proc:/proc/mdstat]` section.

   Make sure that the `enabled` option is set to `yes`:

   ```
   [plugin:proc:/proc/mdstat]
       # enabled = yes
   ```

   If you make any changes to the configuration, restart the Netdata Agent for the changes to take effect:

   ```
   sudo systemctl restart netdata
   ```

3. Check the md driver data collection:

   After verifying the Netdata configuration, check if data collection is successful. On the Netdata dashboard, go to the "Disks" section, and look for "mdX" (where "X" is a number) in the list of available disks. If you can see the charts for your RAID array(s), it means data collection is working correctly.

4. Investigate system logs:

   If the issue persists, check the system logs for any errors or messages related to the md driver or Netdata Agent. You can use `journalctl` for this purpose:

   ```
   journalctl -u netdata
   ```

   Look for any error messages or warnings that could indicate the cause of the problem.

### Useful resources

1. [Linux RAID: A Quick Guide](https://www.cyberciti.biz/tips/linux-raid-increase-resync-rebuild-speed.html)
2. [Netdata Agent Configuration Guide](/src/daemon/config/README.md)
