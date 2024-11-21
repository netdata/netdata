### Understand the alert

This alert is related to the Netdata Exporting engine, which calculates the number of seconds since the last successful buffering of exporting data. If you receive this alert, it means the exporting engine failed to buffer metrics for a while, and some metrics were lost during exporting. There might be issues with the exporting destination being down or unreachable.

### Troubleshoot the alert

1. Check the exporting destination status and accessibility: If the exporting destination (e.g. a remote server or database) is down or unreachable, your priority should be to fix the connection issue or bring the destination back up.

2. Investigate short-term network availability problems: Short-term network connectivity issues might cause temporary errors in the exporting process. You may want to check and monitor your network to confirm this is the case and fix any issues.

3. Increase the `buffer on failures` value in `exporting.conf`: You can try to prevent short-term problems from causing alert issues by increasing the `buffer on failures` value in the `exporting.conf` file. To do this, edit the configuration file, find the parameter `buffer on failures`, and increase its value.
  
   ```
   [exporting:global]
       buffer on failures = new_value
   ```
   Replace `new_value` with the desired number that matches your system requirements.

4. Restart the Netdata Agent: After modifying the `exporting.conf` file, don't forget to restart the Netdata Agent for changes to take effect. Use the following command to restart the Agent: 
   
   ```
   sudo systemctl restart netdata
   ```

5. Monitor the `exporting_last_buffering` alert: After applying the changes, keep monitoring the `exporting_last_buffering` alert to check if the issue is resolved. If the alert continues, further investigate possible issues with the exporting engine or destination.

### Useful resources

1. [Netdata Exporting Reference](/src/exporting/README.md)
