### Understand the alert

The `exporting_metrics_sent` alert is triggered when the Netdata Agent fails to send all metrics to the configured external database server. This could be due to the exporting destination being down, unreachable, or short-term network availability problems.

### Troubleshoot the alert

To troubleshoot this alert, follow these steps:

1. Verify the exporting destination status:

   - Make sure the external database server is up and running.
   - Check if there are any issues with the server, such as high CPU usage, low memory, or a full disk.

2. Check the network connection between the Netdata Agent and the external database server:

   - Use tools like `ping` or `traceroute` to test the connection.
   - Check for any firewall rules that may be blocking the connection.

3. Increase the `buffer on failures` in `exporting.conf`:

   - Open the `exporting.conf` file, which is typically located at `/etc/netdata/exporting.conf`.
   
   - Increase the value of the `buffer on failures` setting to allow for more metrics to be stored when network/connectivity issues occur. For example, if the current setting is `10000`, try increasing it to `20000` or higher, depending on your server's available memory.
   
   ```
   [exporting:global]
       buffer on failures = 20000
   ```
   
   - Save and exit the file.
   
   - Restart the Netdata Agent to apply the changes.

4. Review the Netdata Agent logs:

   - Check for any error messages or warnings related to the exporting engine in the Netdata Agent logs (`/var/log/netdata/error.log`).
   
   - Use the information from the logs to troubleshoot any issues you find.

5. Ensure your configuration settings are correct:

   - Double-check your exporting configuration settings (located in `/etc/netdata/exporting.conf`) to ensure they match the requirements of your external database server.

### Useful resources

1. [Netdata Exporting Reference](/src/exporting/README.md)
