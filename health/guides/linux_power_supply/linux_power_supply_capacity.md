### Understand the alert

The `linux_power_supply_capacity` alert is triggered when the remaining power supply capacity of a Linux system is low. A warning state occurs when the capacity falls below 10%, and a critical state occurs when it falls below 5%. This alert indicates that the system may run out of power and shut down soon.

### Troubleshoot the alert

1. **Restore power**: Connect the system to a power source to recharge the battery and prevent an unexpected shutdown.

2. **Check battery health**: Inspect the health of the system's battery. If the capacity is consistently low or degrading, consider replacing the battery.

3. **Consider a UPS**: If your system experiences frequent power interruptions, you may want to integrate an uninterruptible power supply (UPS) to provide temporary power and prevent system shutdowns.

4. **Monitor power supply metrics**: Keep an eye on power supply metrics, such as remaining capacity and charge/discharge rate, to ensure the system is functioning optimally.

### Useful resources

1. [Battery Health Monitoring on Linux](https://wiki.archlinux.org/title/Laptop#Battery)
2. [Monitoring Power Supply on Linux](https://askubuntu.com/questions/69556/how-to-check-battery-status-using-terminal)
