### Understand the alert

This alert presents the average `I/O latency` over the last 10 seconds. `I/O latency` is the time that is required to complete a single I/O operation on a block device.

This alert might indicate that your disk is under high load, or that the disk is slow.

### Troubleshoot the alert

1. Check per-process I/O usage:
   
   Use `iotop` to see the processes that are the main I/O consumers:

   ```
   sudo iotop
   ```

   If you don't have `iotop` installed, then [install it](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)

2. Analyze the running processes:

   Investigate the top I/O consumers and determine if these processes are expected to consume that much I/O, or if there might be an issue with these processes.

3. Minimize the load by closing any unnecessary main consumer processes:

   If you find that any unnecessary or unexpected processes are heavily utilizing your disk, try stopping or closing those processes to reduce the load on the disk. Always double-check if the process you want to close is necessary.

4. Verify your disk health:

   Make sure your disk is not facing any hardware issues or failures. For this, you can use the `smartmontools` package, which contains the `smartctl` utility. If it's not installed, you can [install it](https://www.smartmontools.org/wiki/Download).

   To check the disk health, run:

   ```
   sudo smartctl -a /dev/sdX
   ```

   Replace `/dev/sdX` with the correct disk device identifier (for example, `/dev/sda`).

5. Consider upgrading your disk:

   If your disk consistently experiences high latency and you have already addressed any performance issues with the running processes, consider upgrading your disk to a faster drive (e.g., replace an HDD with an SSD).

### Useful resources

1. [iotop - Monitor Linux Disk I/O Activity](https://www.tecmint.com/iotop-monitor-linux-disk-io-activity-per-process/)
2. [smartmontools - SMART monitoring tools](https://www.smartmontools.org/)
