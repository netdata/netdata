### Understand the alert

This alert is based on the `upsd_10min_ups_load` metric, which measures the average UPS load over the last 10 minutes. If you receive this alert, it means that the load on your UPS is higher than expected, which may lead to an unstable power supply and ungraceful system shutdowns.

### Troubleshoot the alert

1. Verify the UPS load status

   Check the current load on the UPS using the `upsc` command with your UPS identifier:
   ```
   upsc <your_ups_identifier>
   ```
   Look for the `ups.load` metric in the command output to identify the current load percentage.

2. Analyze the connected devices

   Make an inventory of all devices connected to the UPS, including servers, networking devices, and other equipment. Determine if all devices are essential or if some can be moved to another power source or disconnected entirely.

3. Balance the load between multiple UPS units (if available)

   If you have more than one UPS, consider distributing the connected devices across multiple units to balance the load and ensure that each UPS isn't overloaded.

4. Upgrade or replace the UPS

   If necessary, consider upgrading your UPS to a higher capacity model to handle the increased load or replacing the current unit if it's malfunctioning or unable to provide the required power.

5. Monitor power usage trends

   Regularly review your power usage patterns and system logs, and take action to prevent load spikes that could trigger the `nut_10min_ups_load` alert.

6. Optimize device power consumption

   Implement power-saving strategies for connected devices, such as enabling power-saving modes, reducing CPU usage, or using power-efficient networking equipment.

### Useful resources

1. [NUT user manual](https://networkupstools.org/docs/user-manual.chunked/index.html)
2. [Five steps to reduce UPS energy consumption](https://sp.ts.fujitsu.com/dmsp/Publications/public/wp-reduce-ups-energy-consumption-ww-en.pdf)
