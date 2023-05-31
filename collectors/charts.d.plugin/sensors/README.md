# Linux machine sensors collector

Use this collector when `lm-sensors` doesn't work on your device (e.g. for RPi temperatures).
For all other cases use the [Python collector](https://github.com/netdata/netdata/blob/master/collectors/python.d.plugin/sensors), which supports multiple
jobs, is more efficient and performs calculations on top of the kernel provided values.

This plugin will provide charts for all configured system sensors, by reading sensors directly from the kernel.
The values graphed are the raw hardware values of the sensors.

The plugin will create Netdata charts for:

1. **Temperature**
2. **Voltage**
3. **Current**
4. **Power**
5. **Fans Speed**
6. **Energy**
7. **Humidity**

One chart for every sensor chip found and each of the above will be created.

## Enable the collector

If using [our official native DEB/RPM packages](https://github.com/netdata/netdata/blob/master/packaging/installer/methods/packages.md), make sure `netdata-plugin-chartsd` is installed.

The `sensors` collector is disabled by default.

To enable the collector, you need to edit the configuration file of `charts.d/sensors.conf`. You can do so by using the `edit config` script.

> ### Info
>
> To edit configuration files in a safe way, we provide the [`edit config` script](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#use-edit-config-to-edit-configuration-files) located in your [Netdata config directory](https://github.com/netdata/netdata/blob/master/docs/configure/nodes.md#the-netdata-config-directory) (typically is `/etc/netdata`) that creates the proper file and opens it in an editor automatically.
> It is recommended to use this way for configuring Netdata.
>
> Please also note that after most configuration changes you will need to [restart the Agent](https://github.com/netdata/netdata/blob/master/docs/configure/start-stop-restart.md) for the changes to take effect.

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d.conf
```

You need to uncomment the regarding `sensors`, and set the value to `force`.

```shell
# example=force
sensors=force
```

## Configuration

Edit the `charts.d/sensors.conf` configuration file using `edit-config`:

```bash
cd /etc/netdata   # Replace this path with your Netdata config directory, if different
sudo ./edit-config charts.d/sensors.conf
```

This is the internal default for `charts.d/sensors.conf`

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
