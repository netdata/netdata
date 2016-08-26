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

It produces following charts:

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

# cpufreq

Module shows current cpu frequency by looking at appropriate files in /sys/devices

**Requirement:**
Processor which presents data scaling frequency data

It produces one chart with multiple lines (one line per core).

### configuration

Sample:

```yaml
sys_dir: "/sys/devices"
```

If no configuration is given, module will search for `scaling_cur_freq` files in `/sys/devices` directory.
Directory is also prefixed with `NETDATA_HOST_PREFIX` if specified.

---

# dovecot

This module provides statistics information from dovecot server. 
Statistics are taken from dovecot socket by executing `EXPORT global` command.
More information about dovecot stats can be found on [project wiki page.](http://wiki2.dovecot.org/Statistics)

**Requirement:**
Dovecot unix socket with R/W permissions for user netdata or dovecot with configured TCP/IP socket.
 
Module gives information with following charts:

1. **logins and sessions**
 * logins
 * active sessions

2. **commands** - number of IMAP commands 
 * commands
 
3. **Faults**
 * minor
 * major
 
4. **Context Switches** 
 * volountary
 * involountary
 
5. **disk** in bytes/s
 * read
 * write
 
6. **bytes** in bytes/s
 * read
 * write
 
7. **number of syscalls** in syscalls/s
 * read
 * write

8. **lookups** - number of lookups per second
 * path
 * attr

9. **hits** - number of cache hits 
 * hits

10. **attempts** - authorization attemts
 * success
 * failure

11. **cache** - cached authorization hits
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

# exim

Simple module executing `exim -bpc` to grab exim queue. 
This command can take a lot of time to finish its execution thus it is not recommended to run it every second.

It produces only one chart:

1. **Exim Queue Emails**
 * emails

Configuration is not needed.

---

# hddtemp
 
Module monitors disk temperatures from one or more hddtemp daemons

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

This module will monitor one or more nginx servers depending on configuration. 

**Requirements:**
 * nginx with configured `stub_status`

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
