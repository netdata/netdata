# nut

The plugin will collect UPS data for all UPSes configured in the system.

The following charts will be created:

1.  **UPS Charge**

-   percentage changed

2.  **UPS Battery Voltage**

-   current voltage
-   high voltage
-   low voltage
-   nominal voltage

3.  **UPS Input Voltage**

-   current voltage
-   fault voltage
-   nominal voltage

4.  **UPS Input Current**

-   nominal current

5.  **UPS Input Frequency**

-   current frequency
-   nominal frequency

6.  **UPS Output Voltage**

-   current voltage

7.  **UPS Load**

-   current load

8.  **UPS Temperature**

-   current temperature

## configuration

This is the internal default for `/etc/netdata/nut.conf`

```sh
# a space separated list of UPS names
# if empty, the list returned by 'upsc -l' will be used
nut_ups=

# how frequently to collect UPS data
nut_update_every=2
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fnut%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
