# apcupsd_last_collected_secs

**Power Supply | UPS device**

This is an alert about your American Power Conversion (APC) uninterruptible power supply (UPS) device. 
The Netdata Agent monitors the number of seconds since
the last successful data collection by querying the `apcaccess` tool. This alert indicates that no
data collection has taken place for some time.

### Troubleshooting section:

<details>
<summary>Check the APCU daemon </summary>

1. Check the status of the APCU daemon
    ```
    root@netdata $ systemctl status apcupsd
    ```
   
2. Check for obvious and common errors.


3. Restart the APCU daemon, if needed
    ```
    root@netdata $ systemctl restart apcupsd
    ```
</details>
