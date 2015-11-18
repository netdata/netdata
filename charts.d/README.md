The following charts.d plugins are supported:

# squid

The plugin will monitor a squid server.

It will produce 4 charts:

1. **Squid Client Bandwidth** in kbps

 * in
 * out
 * hits

2. **Squid Client Requests** in requests/sec

 * requests
 * hits
 * errors

3. **Squid Server Bandwidth** in kbps

 * in
 * out

4. **Squid Server Requests** in requests/sec

 * requests
 * errors

### autoconfig

The plugin will by itself detect squid servers running on
localhost, on ports 3128 or 8080.

It will attempt to download URLs in the form:

- `cache_object://HOST:PORT/counters`
- `/squid-internal-mgr/counters`

If any succeeds, it will use this.

### configuration

If you need to configure it by hand, create the file
`/etc/netdata/squid.conf` with the following variables:

- `squid_host=IP` the IP of the squid host
- `squid_port=PORT` the port the squid is listening
- `squid_url="URL"` the URL with the statistics to be fetched from squid
- `squid_timeout=SECONDS` how much time we should wait for squid to respond
- `squid_update_every=SECONDS` the frequency of the data collection

Example `/etc/netdata/squid.conf`:

```sh
squid_host=127.0.0.1
squid_port=3128
squid_url="cache_object://127.0.0.1:3128/counters"
squid_timeout=2
squid_update_every=5
```

---

# sensors

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
```

---

# postfix

The plugin will collect the postfix queue size.

It will create two charts:

1. **queue size in emails**
2. **queue size in KB**

### configuration

This is the internal default for `/etc/netdata/postfix.conf`

```sh
# the postqueue command
# if empty, it will use the one found in the system path
postfix_postqueue=

# how frequently to collect queue size
postfix_update_every=15
```

---

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

