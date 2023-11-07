# apcupsd_10min_ups_load

**Power Supply | UPS**

This is an alert about your American Power Conversion (APC) uninterruptible power supply (UPS) device. 
The Netdata Agent calculates the average UPS load over the last 10 minutes. Receiving 
this alert means that your UPS has a very high load. This issue may result
in either your UPS transferring to bypass mode or shutting down as a safety
measure due to overload. You should remove some attached equipment from the UPS.

This alert is triggered in warning state when the average UPS load is between 70-80% and in critical
state when it is between 85-95%.

### Troubleshooting section:

<details>
<summary>Reduce the load on the UPS</summary>

To avoid ungraceful shutdowns of your systems, consider reducing the load on this particular UPS.
To achieve this, consider removing attached devices that are not mission critical.

</details>
