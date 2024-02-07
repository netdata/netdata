### Understand the alert

The `unbound_request_list_dropped` alert indicates that the Unbound DNS resolver is dropping new incoming requests because its request queue is full. This situation may be caused by a high volume of DNS queries, possibly from a Denial of Service (DoS) attack or poor server optimization.

### Troubleshoot the alert

1. **Check the request queue length**: Inspect the Unbound configuration file (usually located at `/etc/unbound/unbound.conf`) and check the `num-queries-per-thread` setting. If the value is too low for your system, you may encounter issues with dropped requests.

2. **Increase the queue length**: If necessary, increase the `num-queries-per-thread` value in the Unbound configuration file. For example, if the current value is 1024, you can try setting it to a higher value, such as 2048 or 4096. Save the changes and restart the Unbound service:

   ```
   sudo systemctl restart unbound
   ```

3. **Monitor dropped requests**: Use the `unbound-control` command to monitor the number of dropped requests in real-time:

   ```
   sudo unbound-control stats_noreset | grep num.requestlist.dropped
   ```

   If you see the dropped requests decreasing, your changes to the `num-queries-per-thread` value may have resolved the issue.

4. **Inspect server logs**: Check the Unbound log file (usually located at `/var/log/unbound.log`) for any suspicious activity or error messages that may indicate the cause of the increased DNS queries.

5. **Check for potential DoS attacks**: Use tools like `iftop`, `nload`, or `nethogs` to monitor network traffic and identify any potential DoS attacks or unusual traffic patterns.

   If you believe your server is experiencing a DoS attack:

   - Investigate the source IP addresses of the high-volume traffic
   - Block malicious traffic using firewall tools like `iptables` or `ufw`
   - Contact your hosting provider, ISP, or network administrator for assistance

6. **Optimize Unbound**: Review the [official Unbound documentation](https://nlnetlabs.nl/documentation/unbound/) and tune the settings in the Unbound configuration file to ensure optimal performance for your specific environment.

### Useful resources

1. [Unbound Official Documentation](https://nlnetlabs.nl/documentation/unbound/)
2. [How to set up a DNS Resolver with Unbound](https://calomel.org/unbound_dns.html)
