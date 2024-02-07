### Understand the alert

This alert is related to the charge level of your American Power Conversion (APC) uninterruptible power supply (UPS) device. When the UPS charge level drops below a certain threshold, you receive an alert indicating that the system is running on battery and may shut down if external power is not restored soon.

- Warning state: UPS charge < 100%
- Critical state: UPS charge < 50%

The main purpose of a UPS is to provide a temporary power source to connected devices in case of a power outage. As the battery charge decreases, you need to either restore the power supply or prepare the equipment for a graceful shutdown.

### Troubleshoot the alert

1. Check the UPS charge level and status

   To investigate the current status and charge level of the UPS, you can use the `apcaccess` command which provides information about the APC UPS device.

   ```
   apcaccess
   ```

   Look for the `STATUS` and `BCHARGE` fields in the output.

2. Restore the power supply (if possible)

   If the power outage is temporary or local (e.g. due to a tripped circuit breaker), try to restore the power supply to the UPS by fixing the issue or connecting the UPS to a different power source.

3. Prepare for a graceful shutdown

   If you cannot restore power to the UPS, or if the battery charge is critically low, you should immediately prepare your machine and any connected devices for a graceful shutdown. This will help to avoid data loss or system corruption due to an abrupt shutdown.

   For Linux systems, you can execute the following command to perform a graceful shutdown:

   ```
   sudo shutdown -h +1 "UPS battery is low. The system will shut down in 1 minute."
   ```

   For Windows systems, open a command prompt with admin privileges and execute the following command to perform a graceful shutdown:

   ```
   shutdown /s /t 60 /c "UPS battery is low. The system will shut down in 1 minute."
   ```

4. Monitor UPS and system logs

   Keep an eye on UPS and system logs to detect any issues with the power supply or UPS device. This can help you stay informed about the system's status and troubleshoot any potential problems.

