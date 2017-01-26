# Disclaimer

Every module should be compatible with python2 and python3.
All third party libraries should be installed system-wide or in `python_modules` directory.
Module configurations are written in YAML and **pyYAML is required**.

Every configuration file must have one of two formats:

- Configuration for only one job:

```yaml
update_every : 2 # update frequency
retries      : 1 # how many failures in update() is tolerated
priority     : 20000 # where it is shown on dashboard

other_var1   : bla  # variables passed to module
other_var2   : alb
```

- Configuration for many jobs (ex. mysql):

```yaml
# module defaults:
update_every : 2
retries      : 1
priority     : 20000

local:  # job name
  update_every : 5 # job update frequency
  other_var1   : some_val # module specific variable

other_job: 
  priority     : 5 # job position on dashboard
  retries      : 20 # job retries
  other_var2   : val # module specific variable
```

`update_every`, `retries`, and `priority` are always optional.

---

The following python.d modules are supported:

# apache

This module will monitor one or more apache servers depending on configuration. 

**Requirements:**
 * apache with enabled `mod_status`

It produces the following charts:

1. **Requests** in requests/s
 * requests

2. **Connections**
 * connections

3. **Async Connections**
 * keepalive
 * closing
 * writing
 
4. **Bandwidth** in kilobytes/s
 * sent
 
5. **Workers**
 * idle
 * busy
 
6. **Lifetime Avg. Requests/s** in requests/s
 * requests_sec
 
7. **Lifetime Avg. Bandwidth/s** in kilobytes/s
 * size_sec
 
8. **Lifetime Avg. Response Size** in bytes/request
 * size_req

### configuration

Needs only `url` to server's `server-status?auto`

Here is an example for 2 servers:

```yaml
update_every : 10
priority     : 90100

local:
  url      : 'http://localhost/server-status?auto'
  retries  : 20

remote:
  url          : 'http://www.apache.org/server-status?auto'
  update_every : 5
  retries      : 4
```

Without configuration, module attempts to connect to `http://localhost/server-status?auto`

---

# apache_cache

Module monitors apache mod_cache log and produces only one chart:

**cached responses** in percent cached
 * hit
 * miss
 * other
 
### configuration

Sample:

```yaml
update_every : 10
priority     : 120000
retries      : 5
log_path     : '/var/log/apache2/cache.log'
```

If no configuration is given, module will attempt to read log file at `/var/log/apache2/cache.log`

---

# bind_rndc

Module parses bind dump file to collect real-time performance metrics

**Requirements:**
 * Version of bind must be 9.6 +
 * Netdata must have permissions to run `rndc status`

It produces:

1. **Name server statistics**
 * requests
 * responses
 * success
 * auth_answer
 * nonauth_answer
 * nxrrset
 * failure
 * nxdomain
 * recursion
 * duplicate
 * rejections
 
2. **Incoming queries**
 * RESERVED0
 * A
 * NS
 * CNAME
 * SOA
 * PTR
 * MX
 * TXT
 * X25
 * AAAA
 * SRV
 * NAPTR
 * A6
 * DS
 * RSIG
 * DNSKEY
 * SPF
 * ANY
 * DLV
 
3. **Outgoing queries**
 * Same as Incoming queries


### configuration

Sample:

```yaml
local:
  named_stats_path       : '/var/log/bind/named.stats'
```

If no configuration is given, module will attempt to read named.stats file  at `/var/log/bind/named.stats`

---

# cpufreq

This module shows the current CPU frequency as set by the cpufreq kernel
module.

**Requirement:**
You need to have `CONFIG_CPU_FREQ` and (optionally) `CONFIG_CPU_FREQ_STAT`
enabled in your kernel.

This module tries to read from one of two possible locations. On
initialization, it tries to read the `time_in_state` files provided by
cpufreq\_stats. If this file does not exist, or doesn't contain valid data, it
falls back to using the more inaccurate `scaling_cur_freq` file (which only
represents the **current** CPU frequency, and doesn't account for any state
changes which happen between updates).

It produces one chart with multiple lines (one line per core).

### configuration

Sample:

```yaml
sys_dir: "/sys/devices"
```

If no configuration is given, module will search for cpufreq files in `/sys/devices` directory.
Directory is also prefixed with `NETDATA_HOST_PREFIX` if specified.

---

# cpuidle

This module monitors the usage of CPU idle states.

**Requirement:**
Your kernel needs to have `CONFIG_CPU_IDLE` enabled.

It produces one stacked chart per CPU, showing the percentage of time spent in
each state.

---

# dovecot

This module provides statistics information from dovecot server. 
Statistics are taken from dovecot socket by executing `EXPORT global` command.
More information about dovecot stats can be found on [project wiki page.](http://wiki2.dovecot.org/Statistics)

**Requirement:**
Dovecot unix socket with R/W permissions for user netdata or dovecot with configured TCP/IP socket.
 
Module gives information with following charts:

1. **sessions**
 * active sessions

2. **logins**
 * logins

3. **commands** - number of IMAP commands 
 * commands
 
4. **Faults**
 * minor
 * major
 
5. **Context Switches** 
 * volountary
 * involountary
 
6. **disk** in bytes/s
 * read
 * write
 
7. **bytes** in bytes/s
 * read
 * write
 
8. **number of syscalls** in syscalls/s
 * read
 * write

9. **lookups** - number of lookups per second
 * path
 * attr

10. **hits** - number of cache hits 
 * hits

11. **attempts** - authorization attemts
 * success
 * failure

12. **cache** - cached authorization hits
 * hit
 * miss
 
### configuration

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242

localsocket:
  name     : 'local'
  socket   : '/var/run/dovecot/stats'
```

If no configuration is given, module will attempt to connect to dovecot using unix socket localized in `/var/run/dovecot/stats`

---

# elasticsearch

Module monitor elasticsearch performance and health metrics

**Requirements:**
 * python `requests` package.

You need to install it manually. (python-requests or python3-requests depending on the version of python).


It produces:

1. **Search performance** charts:
 * Number of queries, fetches
 * Time spent on queries, fetches
 * Query and fetch latency

2. **Indexing performance** charts:
 * Number of documents indexed, index refreshes, flushes
 * Time spent on indexing, refreshing, flushing
 * Indexing and flushing latency

3. **Memory usage and garbace collection** charts:
 * JVM heap currently in use, commited
 * Count of garbage collections
 * Time spent on garbage collections

4. **Host metrics** charts:
 * Available file descriptors in percent 
 * Opened HTTP connections
 * Cluster communication transport metrics

5. **Queues and rejections** charts:
 * Number of queued/rejected threads in thread pool

6. **Fielddata cache** charts:
 * Fielddata cache size
 * Fielddata evictions and circuit breaker tripped count

7. **Cluster health API** charts:
 * Cluster status
 * Nodes and tasks statistics
 * Shards statistics

8. **Cluster stats API** charts:
 * Nodes statistics
 * Query cache statistics
 * Docs statistics
 * Store statistics
 * Indices and shards statistics

### configuration

Sample:

```yaml
local:
  host               :  'ipaddress'   # Server ip address or hostname
  port               : 'password'     # Port on which elasticsearch listed
  cluster_health     :  True/False    # Calls to cluster health elasticsearch API. Enabled by default.
  cluster_stats      :  True/False    # Calls to cluster stats elasticsearch API. Enabled by default.
```

If no configuration is given, module will fail to run.

---

# exim

Simple module executing `exim -bpc` to grab exim queue. 
This command can take a lot of time to finish its execution thus it is not recommended to run it every second.

It produces only one chart:

1. **Exim Queue Emails**
 * emails

Configuration is not needed.

---

# fail2ban

Module monitor fail2ban log file to show all bans for all active jails 

**Requirements:**
 * fail2ban.log file MUST BE readable by netdata (A good idea is to add  **create 0640 root netdata** to fail2ban conf at logrotate.d)
 
It produces one chart with multiple lines (one line per jail)
 
### configuration

Sample:

```yaml
local:
 log_path: '/var/log/fail2ban.log'
 conf_path: '/etc/fail2ban/jail.local'
 exclude: 'dropbear apache'
```
If no configuration is given, module will attempt to read log file at `/var/log/fail2ban.log` and conf file at `/etc/fail2ban/jail.local`.
If conf file is not found default jail is `ssh`.

---

# freeradius

Uses the `radclient` command to provide freeradius statistics. It is not recommended to run it every second.

It produces:

1. **Authentication counters:**
 * access-accepts
 * access-rejects
 * auth-dropped-requests
 * auth-duplicate-requests
 * auth-invalid-requests
 * auth-malformed-requests
 * auth-unknown-types

2. **Accounting counters:** [optional]
 * accounting-requests
 * accounting-responses
 * acct-dropped-requests
 * acct-duplicate-requests
 * acct-invalid-requests
 * acct-malformed-requests
 * acct-unknown-types

3. **Proxy authentication counters:** [optional]
 * proxy-access-accepts
 * proxy-access-rejects
 * proxy-auth-dropped-requests
 * proxy-auth-duplicate-requests
 * proxy-auth-invalid-requests
 * proxy-auth-malformed-requests
 * proxy-auth-unknown-types

4. **Proxy accounting counters:** [optional]
 * proxy-accounting-requests
 * proxy-accounting-responses
 * proxy-acct-dropped-requests
 * proxy-acct-duplicate-requests
 * proxy-acct-invalid-requests
 * proxy-acct-malformed-requests
 * proxy-acct-unknown-typesa


### configuration

Sample:

```yaml
local:
  host       : 'localhost'
  port       : '18121'
  secret     : 'adminsecret'
  acct       : False # Freeradius accounting statistics.
  proxy_auth : False # Freeradius proxy authentication statistics. 
  proxy_acct : False # Freeradius proxy accounting statistics.
```

**Freeradius server configuration:**

The configuration for the status server is automatically created in the sites-available directory.
By default, server is enabled and can be queried from every client. 
FreeRADIUS will only respond to status-server messages, if the status-server virtual server has been enabled.

To do this, create a link from the sites-enabled directory to the status file in the sites-available directory:
 * cd sites-enabled
 * ln -s ../sites-available/status status

and restart/reload your FREERADIUS server.

---

# haproxy

Module monitors frontend and backend metrics such as bytes in, bytes out, sessions current, sessions in queue current.
And health metrics such as backend servers status (server check should be used).

Plugin can obtain data from url **OR** unix socket.

**Requirement:**
Socket MUST be readable AND writable by netdata user.

It produces:

1. **Frontend** family charts
 * Kilobytes in/s 
 * Kilobytes out/s
 * Sessions current
 * Sessions in queue current

2. **Backend** family charts
 * Kilobytes in/s 
 * Kilobytes out/s
 * Sessions current
 * Sessions in queue current

3. **Health** chart
 * number of failed servers for every backend (in DOWN state)


### configuration

Sample:

```yaml
via_url:
  user       : 'username' # ONLY IF stats auth is used
  pass       : 'password' # # ONLY IF stats auth is used
  url     : 'http://ip.address:port/url;csv;norefresh'
```

OR

```yaml
via_socket:
  socket       : 'path/to/haproxy/sock'
```

If no configuration is given, module will fail to run.

---

# hddtemp
 
Module monitors disk temperatures from one or more hddtemp daemons.

**Requirement:**
Running `hddtemp` in daemonized mode with access on tcp port

It produces one chart **Temperature** with dynamic number of dimensions (one per disk)

### configuration

Sample:

```yaml
update_every: 3
host: "127.0.0.1"
port: 7634
```

If no configuration is given, module will attempt to connect to hddtemp daemon on `127.0.0.1:7634` address

---

# IPFS

Module monitors [IPFS](https://ipfs.io) basic information.

1. **Bandwidth** in kbits/s
 * in
 * out
 
2. **Peers**
 * peers
 
### configuration

Only url to IPFS server is needed. 

Sample:

```yaml
localhost:
  name : 'local'
  url  : 'http://localhost:5001'
```

---

# isc_dhcpd

Module monitor leases database to show all active leases for given pools.

**Requirements:**
 * dhcpd leases file MUST BE readable by netdata
 * pools MUST BE in CIDR format

It produces:

1. **Pools utilization** Aggregate chart for all pools.
 * utilization in percent

2. **Total leases**
 * leases (overall number of leases for all pools)
 
3. **Active leases** for every pools
  * leases (number of active leases in pool)

  
### configuration

Sample:

```yaml
local:
  leases_path       : '/var/lib/dhcp/dhcpd.leases'
  pools       : '192.168.3.0/24 192.168.4.0/24 192.168.5.0/24'
```

In case of python2 you need to  install `py2-ipaddress` to make plugin work.
The module will not work If no configuration is given.

---


# mdstat

Module monitor /proc/mdstat

It produces:

1. **Health** Number of failed disks in every array (aggregate chart).
 
2. **Disks stats** 
 * total (number of devices array ideally would have)
 * inuse (number of devices currently are in use)

3. **Current status**
 * resync in percent
 * recovery in percent
 * reshape in percent
 * check in percent
 
4. **Operation status** (if resync/recovery/reshape/check is active)
 * finish in minutes
 * speed in megabytes/s
  
### configuration
No configuration is needed.

---

# memcached

Memcached monitoring module. Data grabbed from [stats interface](https://github.com/memcached/memcached/wiki/Commands#stats).

1. **Network** in kilobytes/s
 * read
 * written
 
2. **Connections** per second
 * current
 * rejected
 * total
 
3. **Items** in cluster
 * current
 * total
 
4. **Evicted and Reclaimed** items
 * evicted
 * reclaimed
 
5. **GET** requests/s
 * hits
 * misses

6. **GET rate** rate in requests/s
 * rate

7. **SET rate** rate in requests/s
 * rate
 
8. **DELETE** requests/s
 * hits
 * misses

9. **CAS** requests/s
 * hits
 * misses
 * bad value
 
10. **Increment** requests/s
 * hits
 * misses
 
11. **Decrement** requests/s
 * hits
 * misses
 
12. **Touch** requests/s
 * hits
 * misses
 
13. **Touch rate** rate in requests/s
 * rate
 
### configuration

Sample:

```yaml
localtcpip:
  name     : 'local'
  host     : '127.0.0.1'
  port     : 24242
```

If no configuration is given, module will attempt to connect to memcached instance on `127.0.0.1:11211` address.

---

# mysql

Module monitors one or more mysql servers

**Requirements:**
 * python library [MySQLdb](https://github.com/PyMySQL/mysqlclient-python) (faster) or [PyMySQL](https://github.com/PyMySQL/PyMySQL) (slower)

It will produce following charts (if data is available):

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

You can provide, per server, the following:

1. username which have access to database (deafults to 'root')
2. password (defaults to none)
3. mysql my.cnf configuration file
4. mysql socket (optional)
5. mysql host (ip or hostname)
6. mysql port (defaults to 3306)

Here is an example for 3 servers:

```yaml
update_every : 10
priority     : 90100
retries      : 5

local:
  'my.cnf'   : '/etc/mysql/my.cnf'
  priority   : 90000

local_2:
  user     : 'root'
  pass : 'blablablabla'
  socket   : '/var/run/mysqld/mysqld.sock'
  update_every : 1

remote:
  user     : 'admin'
  pass : 'bla'
  host     : 'example.org'
  port     : 9000
  retries  : 20
```

If no configuration is given, module will attempt to connect to mysql server via unix socket at `/var/run/mysqld/mysqld.sock` without password and with username `root`

---

# nginx

This module will monitor one or more nginx servers depending on configuration. Servers can be either local or remote. 

**Requirements:**
 * nginx with configured 'ngx_http_stub_status_module'
 * 'location /stub_status'

Example nginx configuration can be found in 'python.d/nginx.conf'

It produces following charts:

1. **Active Connections**
 * active

2. **Requests** in requests/s
 * requests

3. **Active Connections by Status**
 * reading
 * writing
 * waiting
 
4. **Connections Rate** in connections/s
 * accepts
 * handled
 
### configuration

Needs only `url` to server's `stub_status`

Here is an example for local server:

```yaml
update_every : 10
priority     : 90100

local:
  url     : 'http://localhost/stub_status'
  retries : 10
```

Without configuration, module attempts to connect to `http://localhost/stub_status`

---

# nginx_log

Module monitors nginx access log and produces only one chart:

1. **nginx status codes** in requests/s
 * 2xx
 * 3xx
 * 4xx
 * 5xx

### configuration

Sample for two vhosts:

```yaml
site_A:
  path: '/var/log/nginx/access-A.log'

site_B:
  name: 'local'
  path: '/var/log/nginx/access-B.log'
```

When no configuration file is found, module tries to parse `/var/log/nginx/access.log` file.

---

# ovpn_status_log

Module monitor openvpn-status log file. 

**Requirements:**

 * If you are running multiple OpenVPN instances out of the same directory, MAKE SURE TO EDIT DIRECTIVES which create output files
 so that multiple instances do not overwrite each other's output files.

 * Make sure NETDATA USER CAN READ openvpn-status.log

 * Update_every interval MUST MATCH interval on which OpenVPN writes operational status to log file.
 
It produces:

1. **Users** OpenVPN active users
 * users
 
2. **Traffic** OpenVPN overall bandwidth usage in kilobit/s
 * in
 * out
 
### configuration

Sample:

```yaml
default
 log_path     : '/var/log/openvpn-status.log'
```

---

# phpfpm

This module will monitor one or more php-fpm instances depending on configuration. 

**Requirements:**
 * php-fpm with enabled `status` page
 * access to `status` page via web server
 
It produces following charts:

1. **Active Connections**
 * active
 * maxActive
 * idle

2. **Requests** in requests/s
 * requests
 
3. **Performance**
 * reached
 * slow
 
### configuration

Needs only `url` to server's `status`
 
Here is an example for local instance:

```yaml
update_every : 3
priority     : 90100

local:
  url     : 'http://localhost/status'
  retries : 10
```

Without configuration, module attempts to connect to `http://localhost/status`

---

# postfix

Simple module executing `postfix -p` to grab postfix queue.

It produces only two charts:

1. **Postfix Queue Emails**
 * emails
 
2. **Postfix Queue Emails Size** in KB
 * size

Configuration is not needed.

---

# redis

Get INFO data from redis instance.

Following charts are drawn:

1. **Operations** per second
 * operations

2. **Hit rate** in percent
 * rate

3. **Memory utilization** in kilobytes
 * total
 * lua

4. **Database keys** 
 * lines are creates dynamically based on how many databases are there
 
5. **Clients**
 * connected
 * blocked
 
6. **Slaves**
 * connected
 
### configuration

```yaml
socket:
  name     : 'local'
  socket   : '/var/lib/redis/redis.sock'

localhost:
  name     : 'local'
  host     : 'localhost'
  port     : 6379
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:6379`.

---

# sensors

System sensors information.

Charts are created dynamically.

### configuration

For detailed configuration information please read [`sensors.conf`](https://github.com/firehol/netdata/blob/master/conf.d/python.d/sensors.conf) file.

---

# squid

This module will monitor one or more squid instances depending on configuration.

It produces following charts:

1. **Client Bandwidth** in kilobits/s
 * in
 * out
 * hits

2. **Client Requests** in requests/s
 * requests
 * hits
 * errors

3. **Server Bandwidth** in kilobits/s
 * in
 * out
 
4. **Server Requests** in requests/s
 * requests
 * errors
 
### configuration

```yaml
priority     : 50000

local:
  request : 'cache_object://localhost:3128/counters'
  host    : 'localhost'
  port    : 3128
```

Without any configuration module will try to autodetect where squid presents its `counters` data
 
---

# tomcat

Present tomcat containers memory utilization.

Charts:

1. **Requests** per second
 * accesses

2. **Volume** in KB/s
 * volume

3. **Threads**
 * current
 * busy
 
4. **JVM Free Memory** in MB
 * jvm
 
### configuration

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:8080/manager/status?XML=true'
  user : 'tomcat_username'
  pass : 'secret_tomcat_password'
```

Without configuration, module attempts to connect to `http://localhost:8080/manager/status?XML=true`, without any credentials. 
So it will probably fail.

--- 

# varnish cache

Module uses the `varnishstat` command to provide varnish cache statistics.

It produces:

1. **Client metrics**
 * session accepted
 * session dropped
 * good client requests received

2. **All history hit rate ratio**
 * cache hits in percent
 * cache miss in percent
 * cache hits for pass percent

3. **Curent poll hit rate ratio**
 * cache hits in percent
 * cache miss in percent
 * cache hits for pass percent

4. **Thread-related metrics** (only for varnish version 4+)
 * total number of threads
 * threads created
 * threads creation failed
 * threads hit max
 * length os session queue
 * sessions queued for thread

5. **Backend health**
 * backend conn. success
 * backend conn. not attempted
 * backend conn. too many
 * backend conn. failures
 * backend conn. reuses
 * backend conn. recycles
 * backend conn. retry
 * backend requests made

6. **Memory usage**
 * memory available in megabytes
 * memory allocated in megabytes

7. **Problems summary**
 * session dropped
 * session accept failures
 * session pipe overflow
 * backend conn. not attempted
 * fetch failed (all causes)
 * backend conn. too many
 * threads hit max
 * threads destroyed
 * length of session queue
 * HTTP header overflows
 * ESI parse errors
 * ESI parse warnings

8. **Uptime**
 * varnish instance uptime in seconds

### configuration

No configuration is needed.

---
