### Understand the alert

This alert is related to your American Power Conversion (APC) uninterruptible power supply (UPS) device. The Netdata Agent monitors the number of seconds since the last successful data collection by querying the `apcaccess` tool. If you receive this alert, it means that no data collection has taken place for some time, which might indicate a problem with the APC UPS device or connection.

### Troubleshoot the alert

1. Verify the `apcaccess` tool is installed and functioning properly
   ```
   $ apcaccess status
   ```
   This command should provide you with a status display of the UPS. If the command is not found, you may need to install the `apcaccess` tool.

2. Check the APC UPS daemon

   a. Check the status of the APC UPS daemon
   ```
   $ systemctl status apcupsd
   ```

   b. Check for obvious and common errors, such as wrong device path, incorrect permissions, or configuration issues in `/etc/apcupsd/apcupsd.conf`.

   c. If needed, restart the APC UPS daemon
   ```
   $ systemctl restart apcupsd
   ```

3. Inspect system logs

   Check the system logs for any error messages related to APC UPS or `apcupsd`, which might give more insights into the issue.

4. Verify UPS Connection

   Ensure that the UPS device is properly connected to your server, both physically (USB/Serial) and in the configuration file (`/etc/apcupsd/apcupsd.conf`).

5. Update Netdata configuration

   If the issue is still not resolved, you can try updating the Netdata configuration file for the `apcupsd_last_collected_secs` collector.

6. Check your UPS device

   If all previous steps have been completed and the issue persists, your UPS device might be faulty. Consider contacting the manufacturer for support or replace the device with a known-good unit.

### Useful resources

1. [Netdata - APC UPS monitoring](/src/collectors/charts.d.plugin/apcupsd/integrations/apc_ups.md)
2. [`apcupsd` - Power management and control software for APC UPS](https://github.com/apcupsd/apcupsd)
