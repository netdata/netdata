# Disclaimer

**Python support is experimental and implementation may change in the future**

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
  password : 'blablablabla'
  socket   : '/var/run/mysqld/mysqld.sock'
  update_every : 1

remote:
  user     : 'admin'
  password : 'bla'
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