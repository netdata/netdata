# sensors

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/sensors) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

> Unlike the python one, this module can collect temperature on RPi.


The plugin will provide charts for all configured system sensors

> This plugin is reading sensors directly from the kernel.
> The `lm-sensors` package is able to perform calculations on the
> kernel provided values, this plugin will not perform.
> So, the values graphed, are the raw hardware values of the sensors.

The plugin will create netdata charts for:

1. **Temperature**
2. **Voltage**
3. **Current**
4. **Power**
5. **Fans Speed**
6. **Energy**
7. **Humidity**

One chart for every sensor chip found and each of the above will be created.

### configuration

This is the internal default for `/etc/netdata/sensors.conf`

```sh
# the directory the kernel keeps sensor data
sensors_sys_dir="${NETDATA_HOST_PREFIX}/sys/devices"

# how deep in the tree to check for sensor data
sensors_sys_depth=10

# if set to 1, the script will overwrite internal
# script functions with code generated ones
# leave to 1, is faster
sensors_source_update=1

# how frequently to collect sensor data
# the default is to collect it at every iteration of charts.d
sensors_update_every=

# array of sensors which are excluded
# the default is to include all
sensors_excluded=()
```

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fsensors%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
