# nut_last_collected_secs

**Power Supply | UPS**

Network UPS Tools (NUT) is a suite of software component designed to monitor power devices, such as
uninterruptible power supplies, power distribution units, solar controllers and servers power supply
units.

The Netdata Agent monitors the number of seconds since the last successful data collection

<details>
<summary>References and Sources</summary>

1. [NUT user manual]https://networkupstools.org/docs/user-manual.chunked/index.html

</details>

### Troubleshooting section:

<details>
<summary>Check the upsd server </summary>

1. Check the status of the upsd daemon
    ```
    root@netdata $ systemctl status upsd
    ```

2. Check for obvious and common errors.


3. Restart the daemon if needed
    ```
    root@netdata $ systemctl restart apcupsd
    ```

</details>

<details>
<summary>Diagnose a bad driver</summary>

`upsd` expects the drivers to either update their status regularly or at least answer periodic
queries, called pings. If a driver doesn’t answer, `upsd` will declare it "stale" and no more
information will be provided to the clients.

If upsd complains about staleness when you start it, then either your driver or configuration files
are probably broken. Be sure that the driver is actually running, and that the UPS definition in
[ups.conf(5)](https://networkupstools.org/docs/man/ups.conf.html) is correct. Also make sure that
you start your driver(s) before starting upsd.

Data can also be marked stale if the driver can no longer communicate with the UPS. In this case,
the driver should also provide diagnostic information in the syslog. If this happens, check the
serial or USB cabling, or inspect the network path in the case of a SNMP UPS.
</details>

