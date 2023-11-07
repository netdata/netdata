# apcupsd_ups_charge

**Power Supply | UPS**

This is an alert about your American Power Conversion (APC) uninterruptible power supply (UPS) device. 
The Netdata Agent calculates the average UPS charge over
the last minute. The UPS is running on battery, and it will shut down if external power is not
restored. You should prepare any attached equipment for shutdown.

This alert is triggered in warning state when the average UPS charge is less than 100% and in
critical state when it is less than 50%.

### Troubleshooting section:

<details>
<summary>Prepare your machine for graceful shutdown</summary>

If you can't restore the power supply to this UPC, you should prepare your machine for graceful
shutdown.

</details>
