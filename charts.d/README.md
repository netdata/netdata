The following charts.d plugins are supported:

---

# hddtemp

The plugin will collect temperatures from disks 

It will create one chart with all active disks

1. **temperature in Celsius**

### configuration

hddtemp needs to be running in daemonized mode

```sh
# host with daemonized hddtemp
hddtemp_host="localhost"

# port on which hddtemp is showing data
hddtemp_port="7634"

# array of included disks
# the default is to include all
hddtemp_disks=()
```

---

# libreswan

The plugin will collects bytes-in, bytes-out and uptime for all established libreswan IPSEC tunnels.

The following charts are created, **per tunnel**:

1. **Uptime**

 * the uptime of the tunnel

2. **Traffic**

 * bytes in
 * bytes out

### configuration

Its config file is `/etc/netdata/charts.d/libreswan.conf`.

The plugin executes 2 commands to collect all the information it needs:

```sh
ipsec whack --status
ipsec whack --trafficstatus
```

The first command is used to extract the currently established tunnels, their IDs and their names.
The second command is used to extract the current uptime and traffic.

Most probably user `netdata` will not be able to query libreswan, so the `ipsec` commands will be denied.
The plugin attempts to run `ipsec` as `sudo ipsec ...`, to get access to libreswan statistics.

To allow user `netdata` execute `sudo ipsec ...`, create the file `/etc/sudoers.d/netdata` with this content:

```
netdata ALL = (root) NOPASSWD: /sbin/ipsec whack --status
netdata ALL = (root) NOPASSWD: /sbin/ipsec whack --trafficstatus
```

Make sure the path `/sbin/ipsec` matches your setup (execute `which ipsec` to find the right path).

---

# mysql

The plugin will monitor one or more mysql servers

It will produce the following charts:

1. **Bandwidth** in kbps
 * in
 * out

2. **Queries** in queries/sec
 * queries
 * questions
 * slow queries

3. **Operations** in operations/sec
 * opened tables
 * flush
 * commit
 * delete
 * prepare
 * read first
 * read key
 * read next
 * read prev
 * read random
 * read random next
 * rollback
 * save point
 * update
 * write

4. **Table Locks** in locks/sec
 * immediate
 * waited

5. **Select Issues** in issues/sec
 * full join
 * full range join
 * range
 * range check
 * scan

6. **Sort Issues** in issues/sec
 * merge passes
 * range
 * scan

### configuration

You can configure many database servers, like this:

You can provide, per server, the following:

1. a name, anything you like, but keep it short
2. the mysql command to connect to the server
3. the mysql command line options to be used for connecting to the server

Here is an example for 2 servers:

```sh
mysql_opts[server1]="-h server1.example.com"
mysql_opts[server2]="-h server2.example.com --connect_timeout 2"
```

The above will use the `mysql` command found in the system path.
You can also provide a custom mysql command per server, like this:

```sh
mysql_cmds[server2]="/opt/mysql/bin/mysql"
```

The above sets the mysql command only for server2. server1 will use the system default.

If no configuration is given, the plugin will attempt to connect to mysql server at localhost.


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

# array of sensors which are excluded
# the default is to include all
sensors_excluded=()
```

---

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
