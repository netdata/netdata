# nut

The plugin will collect UPS data for all UPSes configured in the system.

The following charts will be created:

1. **UPS Charge**

 * percentage changed

2. **UPS Battery Voltage**

 * current voltage
 * high voltage
 * low voltage
 * nominal voltage

3. **UPS Input Voltage**

 * current voltage
 * fault voltage
 * nominal voltage

4. **UPS Input Current**

 * nominal current

5. **UPS Input Frequency**

 * current frequency
 * nominal frequency

6. **UPS Output Voltage**

 * current voltage

7. **UPS Load**

 * current load

8. **UPS Temperature**

 * current temperature


### configuration

This is the internal default for `/etc/netdata/nut.conf`

```sh
# a space separated list of UPS names
# if empty, the list returned by 'upsc -l' will be used
nut_ups=

# how frequently to collect UPS data
nut_update_every=2
```

---
