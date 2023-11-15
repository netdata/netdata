### Understand the alert

This alert is related to the Network UPS Tools (NUT) which monitors power devices, such as uninterruptible power supplies, power distribution units, solar controllers, and server power supply units. If you receive this alert, it means that there is an issue with the data collection process and needs troubleshooting to ensure the monitoring process works correctly.

### Troubleshoot the alert

#### Check the upsd server

1. Check the status of the upsd daemon:

   ```
   $ systemctl status upsd
   ```

2. Check for obvious and common errors in the log or output. If any errors are found, resolve them accordingly.

3. Restart the daemon if needed:

   ```
   $ systemctl restart upsd
   ```

#### Diagnose a bad driver

1. `upsd` expects the drivers to either update their status regularly or at least answer periodic queries, called pings. If a driver doesn't answer, `upsd` will declare it "stale" and no more information will be provided to the clients.

2. If upsd complains about staleness when you start it, then either your driver or configuration files are probably broken. Be sure that the driver is actually running, and that the UPS definition in [ups.conf(5)](https://networkupstools.org/docs/man/ups.conf.html) is correct. Also, make sure that you start your driver(s) before starting upsd.

3. Data can also be marked stale if the driver can no longer communicate with the UPS. In this case, the driver should also provide diagnostic information in the syslog. If this happens, check the serial or USB cabling, or inspect the network path in the case of a SNMP UPS.

### Useful resources

1. [NUT User Manual](https://networkupstools.org/docs/user-manual.chunked/index.html)
2. [ups.conf(5)](https://networkupstools.org/docs/man/ups.conf.html)