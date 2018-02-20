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

# beanstalk

Module provides server and tube level statistics:

**Requirements:**
 * `python-beanstalkc`
 * `python-yaml`

**Server statistics:**

1. **Cpu usage** in cpu time
 * user
 * system
 
2. **Jobs rate** in jobs/s
 * total
 * timeouts
 
3. **Connections rate** in connections/s
 * connections
 
4. **Commands rate** in commands/s
 * put
 * peek
 * peek-ready
 * peek-delayed
 * peek-buried
 * reserve
 * use
 * watch
 * ignore
 * delete
 * release
 * bury
 * kick
 * stats
 * stats-job
 * stats-tube
 * list-tubes
 * list-tube-used
 * list-tubes-watched
 * pause-tube
 
5. **Current tubes** in tubes
 * tubes
 
6. **Current jobs** in jobs
 * urgent
 * ready
 * reserved
 * delayed
 * buried
 
7. **Current connections** in connections
 * written
 * producers
 * workers
 * waiting
 
8. **Binlog** in records/s
 * written
 * migrated
 
9. **Uptime** in seconds
 * uptime

**Per tube statistics:**

1. **Jobs rate** in jobs/s
 * jobs
 
2. **Jobs** in jobs
 * using
 * ready
 * reserved
 * delayed
 * buried

3. **Connections** in connections
 * using
 * waiting
 * watching

4. **Commands** in commands/s
 * deletes
 * pauses
 
5. **Pause** in seconds
 * since
 * left

 
### configuration

Sample:

```yaml
host         : '127.0.0.1'
port         : 11300
```

If no configuration is given, module will attempt to connect to beanstalkd on `127.0.0.1:11300` address

---

# bind_rndc

Module parses bind dump file to collect real-time performance metrics

**Requirements:**
 * Version of bind must be 9.6 +
 * Netdata must have permissions to run `rndc stats`

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

# chrony

This module monitors the precision and statistics of a local chronyd server.

It produces:

* frequency
* last offset
* RMS offset
* residual freq
* root delay
* root dispersion
* skew
* system time

**Requirements:**
Verify that user netdata can execute `chronyc tracking`. If necessary, update `/etc/chrony.conf`, `cmdallow`.

### Configuration

Sample:
```yaml
# data collection frequency:
update_every: 1

# chrony query command:
local:
  command: 'chronyc -n tracking'
```

---

# ceph

This module monitors the ceph cluster usage and consuption data of a server.

It produces:

* Cluster statistics (usage, available, latency, objects, read/write rate)
* OSD usage
* OSD latency
* Pool usage
* Pool read/write operations
* Pool read/write rate
* number of objects per pool

**Requirements:**

- `rados` python module
- Granting read permissions to ceph group from keyring file
```shell
# chmod 640 /etc/ceph/ceph.client.admin.keyring
```

### Configuration

Sample:
```yaml
local:
  config_file: '/etc/ceph/ceph.conf'
  keyring_file: '/etc/ceph/ceph.client.admin.keyring'
```

---

# couchdb

This module monitors vital statistics of a local Apache CouchDB 2.x server, including:

* Overall server reads/writes
* HTTP traffic breakdown
  * Request methods (`GET`, `PUT`, `POST`, etc.)
  * Response status codes (`200`, `201`, `4xx`, etc.)
* Active server tasks
* Replication status (CouchDB 2.1 and up only)
* Erlang VM stats
* Optional per-database statistics: sizes, # of docs, # of deleted docs

### Configuration

Sample for a local server running on port 5984:
```yaml
local:
  user: 'admin'
  pass: 'password'
  node: 'couchdb@127.0.0.1'
```

Be sure to specify a correct admin-level username and password.

You may also need to change the `node` name; this should match the value of `-name NODENAME` in your CouchDB's `etc/vm.args` file. Typically this is of the form `couchdb@fully.qualified.domain.name` in a cluster, or `couchdb@127.0.0.1` / `couchdb@localhost` for a single-node server.

If you want per-database statistics, these need to be added to the configuration, separated by spaces:
```yaml
local:
  ...
  databases: 'db1 db2 db3 ...'
```

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
# dns_query_time

This module provides dns query time statistics.

**Requirement:**
* `python-dnspython` package

It produces one aggregate chart or one chart per dns server, showing the query time.

---

# dnsdist

Module monitor dnsdist performance and health metrics.

Following charts are drawn:

1. **Response latency**
 * latency-slow
 * latency100-1000
 * latency50-100
 * latency10-50
 * latency1-10
 * latency0-1

2. **Cache performance**
 * cache-hits
 * cache-misses

3. **ACL events**
 * acl-drops
 * rule-drop
 * rule-nxdomain
 * rule-refused

4. **Noncompliant data**
 * empty-queries
 * no-policy
 * noncompliant-queries
 * noncompliant-responses

5. **Queries**
 * queries
 * rdqueries
 * rdqueries

6. **Health**
 * downstream-send-errors
 * downstream-timeouts
 * servfail-responses
 * trunc-failures

### configuration

```yaml
localhost:
  name : 'local'
  url  : 'http://127.0.0.1:5053/jsonstat?command=stats'
  user : 'username'
  pass : 'password'
  header:
    X-API-Key: 'dnsdist-api-key'
```

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

# go_expvar

---

The `go_expvar` module can monitor any Go application that exposes its metrics with the use of `expvar` package from the Go standard library.

`go_expvar` produces charts for Go runtime memory statistics and optionally any number of custom charts. Please see the [wiki page](https://github.com/firehol/netdata/wiki/Monitoring-Go-Applications) for more info.

For the memory statistics, it produces the following charts:

1. **Heap allocations** in kB
 * alloc: size of objects allocated on the heap
 * inuse: size of allocated heap spans 
 
2. **Stack allocations** in kB
 * inuse: size of allocated stack spans
 
3. **MSpan allocations** in kB
 * inuse: size of allocated mspan structures
 
4. **MCache allocations** in kB
 * inuse: size of allocated mcache structures
 
5. **Virtual memory** in kB
 * sys: size of reserved virtual address space
 
6. **Live objects**
 * live: number of live objects in memory
 
7. **GC pauses average** in ns
 * avg: average duration of all GC stop-the-world pauses
 
### configuration
 
Please see the [wiki page](https://github.com/firehol/netdata/wiki/Monitoring-Go-Applications#using-netdata-go_expvar-module) for detailed info about module configuration.
 
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

# httpcheck

Module monitors remote http server for availability and response time.

Following charts are drawn per job:

1. **Response time** ms
 * Time in 0.1 ms resolution in which the server responds. When connection is successful,
   it will be at least 0.1 ms, even if the measured time is lower than 0.1 ms.
   If it is 0.0 ms, the connection failed with error code >=3 (useful for API)

2. **Error code** int
 * One of:
   - 0: Connection successful
   - 1: Content does not satisfy regex pattern.
   - 2: HTTP status code not accepted
   - 3: Connection failed (port not listening or blocked)
   - 4: Connection timed out (host unreachable?)

### configuration

Sample configuration and their default values.

```yaml
server:
  url: 'http://host:port/path'  # required
  status_accepted:              # optional
    - 200
  timeout: 1                    # optional, supports decimals (e.g. 0.2)
  update_every: 1               # optional
  regex: '.*'                   # optional
  redirect: yes                 # optional
```

### notes

 * The error code chart is intended for health check or for access via API.
 * A system/service/firewall might block netdata's access if a portscan or
   similar is detected.
 * This plugin is meant for simple use cases. For monitoring large networks
   consider a real service monitoring tool.

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

# mongodb

Module monitor mongodb performance and health metrics

**Requirements:**
 * `python-pymongo` package.

You need to install it manually.


Number of charts depends on mongodb version, storage engine and other features (replication):

1. **Read requests**:
 * query
 * getmore (operation the cursor executes to get additional data from query)

2. **Write requests**:
 * insert
 * delete
 * update

3. **Active clients**:
 * readers (number of clients with read operations in progress or queued)
 * writers (number of clients with write operations in progress or queued)

4. **Journal transactions**:
 * commits (count of transactions that have been written to the journal)

5. **Data written to the journal**:
 * volume (volume of data)

6. **Background flush** (MMAPv1):
 * average ms (average time taken by flushes to execute)
 * last ms (time taken by the last flush)

8. **Read tickets** (WiredTiger):
 * in use (number of read tickets in use)
 * available (number of available read tickets remaining)

9. **Write tickets** (WiredTiger):
 * in use (number of write tickets in use)
 * available (number of available write tickets remaining)

10. **Cursors**:
 * opened (number of cursors currently opened by MongoDB for clients)
 * timedOut (number of cursors that have timed)
 * noTimeout (number of open cursors with timeout disabled)

11. **Connections**:
 * connected (number of clients currently connected to the database server)
 * unused (number of unused connections available for new clients)

12. **Memory usage metrics**:
 * virtual
 * resident (amount of memory used by the database process)
 * mapped
 * non mapped

13. **Page faults**:
 * page faults (number of times MongoDB had to request from disk)

14. **Cache metrics** (WiredTiger):
 * percentage of bytes currently in the cache (amount of space taken by cached data)
 * percantage of tracked dirty bytes in the cache (amount of space taken by dirty data)

15. **Pages evicted from cache** (WiredTiger):
 * modified
 * unmodified

16. **Queued requests**:
 * readers (number of read request currently queued)
 * writers (number of write request currently queued)

17. **Errors**:
 * msg (number of message assertions raised)
 * warning (number of warning assertions raised)
 * regular (number of regular assertions raised)
 * user (number of assertions corresponding to errors generated by users)

18. **Storage metrics** (one chart for every database)
 * dataSize (size of all documents + padding in the database)
 * indexSize (size of all indexes in the database)
 * storageSize (size of all extents in the database)

19. **Documents in the database** (one chart for all databases)
 * documents (number of objects in the database among all the collections)

20. **tcmalloc metrics**
 * central cache free
 * current total thread cache
 * pageheap free
 * pageheap unmapped
 * thread cache free
 * transfer cache free
 * heap size

21. **Commands total/failed rate**
 * count
 * createIndex
 * delete
 * eval
 * findAndModify
 * insert

22. **Locks metrics** (acquireCount metrics - number of times the lock was acquired in the specified mode)
 * Global lock
 * Database lock
 * Collection lock
 * Metadata lock
 * oplog lock

23. **Replica set members state**
 * state

24. **Oplog window**
  * window (interval of time between the oldest and the latest entries in the oplog)

25. **Replication lag**
  * member (time when last entry from the oplog was applied for every member)

26. **Replication set member heartbeat latency**
  * member (time when last heartbeat was received from replica set member)


### configuration

Sample:

```yaml
local:
    name : 'local'
    host : '127.0.0.1'
    port : 27017
    user : 'netdata'
    pass : 'netdata'

```

If no configuration is given, module will attempt to connect to mongodb daemon on `127.0.0.1:27017` address

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

# nginx_plus

This module will monitor one or more nginx_plus servers depending on configuration.
Servers can be either local or remote. 

Example nginx_plus configuration can be found in 'python.d/nginx_plus.conf'

It produces following charts:

1. **Requests total** in requests/s
 * total

2. **Requests current** in requests
 * current

3. **Connection Statistics** in connections/s
 * accepted
 * dropped
 
4. **Workers Statistics** in workers
 * idle
 * active
 
5. **SSL Handshakes** in handshakes/s
 * successful
 * failed
 
6. **SSL Session Reuses** in sessions/s
 * reused
 
7. **SSL Memory Usage** in percent
 * usage
 
8. **Processes** in processes
 * respawned
 
For every server zone:

1. **Processing** in requests
 * processing
 
2. **Requests** in requests/s
 * requests
 
3. **Responses** in requests/s
 * 1xx
 * 2xx
 * 3xx
 * 4xx
 * 5xx
 
4. **Traffic** in kilobits/s
 * received
 * sent
 
For every upstream:

1. **Peers Requests** in requests/s
 * peer name (dimension per peer)
 
2. **All Peers Responses** in responses/s
 * 1xx
 * 2xx
 * 3xx
 * 4xx
 * 5xx
 
3. **Peer Responses** in requests/s (for every peer)
 * 1xx
 * 2xx
 * 3xx
 * 4xx
 * 5xx
 
4. **Peers Connections** in active
 * peer name (dimension per peer)
 
5. **Peers Connections Usage** in percent
 * peer name (dimension per peer)
 
6. **All Peers Traffic** in KB
 * received
 * sent
 
7. **Peer Traffic** in KB/s (for every peer)
 * received
 * sent
 
8. **Peer Timings** in ms (for every peer)
 * header
 * response
 
9. **Memory Usage** in percent
 * usage

10. **Peers Status** in state
 * peer name (dimension per peer)
 
11. **Peers Total Downtime** in seconds
 * peer name (dimension per peer)

For every cache:

1. **Traffic** in KB
 * served
 * written
 * bypass
 
2. **Memory Usage** in percent
 * usage
 
### configuration

Needs only `url` to server's `status`

Here is an example for local server:

```yaml
local:
  url     : 'http://localhost/status'
```

Without configuration, module fail to start.

---

# nsd

Module uses the `nsd-control stats_noreset` command to provide `nsd` statistics.

**Requirements:**
 * Version of `nsd` must be 4.0+
 * Netdata must have permissions to run `nsd-control stats_noreset`

It produces:

1. **Queries**
 * queries

2. **Zones**
 * master
 * slave

3. **Protocol**
 * udp
 * udp6
 * tcp
 * tcp6

4. **Query Type**
 * A
 * NS
 * CNAME
 * SOA
 * PTR
 * HINFO
 * MX
 * NAPTR
 * TXT
 * AAAA
 * SRV
 * ANY

5. **Transfer**
 * NOTIFY
 * AXFR

6. **Return Code**
 * NOERROR
 * FORMERR
 * SERVFAIL
 * NXDOMAIN
 * NOTIMP
 * REFUSED
 * YXDOMAIN


Configuration is not needed.

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

# postgres

Module monitors one or more postgres servers.

**Requirements:**

 * `python-psycopg2` package. You have to install to manually.

Following charts are drawn:

1. **Database size** MB
 * size

2. **Current Backend Processes** processes
 * active

3. **Write-Ahead Logging Statistics** files/s
 * total
 * ready
 * done

4. **Checkpoints** writes/s
 * scheduled
 * requested
 
5. **Current connections to db** count
 * connections
 
6. **Tuples returned from db** tuples/s
 * sequential
 * bitmap

7. **Tuple reads from db** reads/s
 * disk
 * cache

8. **Transactions on db** transactions/s
 * commited
 * rolled back

9. **Tuples written to db** writes/s
 * inserted
 * updated
 * deleted
 * conflicts

10. **Locks on db** count per type
 * locks
 
### configuration

```yaml
socket:
  name         : 'socket'
  user         : 'postgres'
  database     : 'postgres'

tcp:
  name         : 'tcp'
  user         : 'postgres'
  database     : 'postgres'
  host         : 'localhost'
  port         : 5432
```

When no configuration file is found, module tries to connect to TCP/IP socket: `localhost:5432`.

---

# powerdns

Module monitor powerdns performance and health metrics.

Following charts are drawn:

1. **Queries and Answers**
 * udp-queries
 * udp-answers
 * tcp-queries
 * tcp-answers

2. **Cache Usage**
 * query-cache-hit
 * query-cache-miss
 * packetcache-hit
 * packetcache-miss

3. **Cache Size**
 * query-cache-size
 * packetcache-size
 * key-cache-size
 * meta-cache-size

4. **Latency**
 * latency

### configuration

```yaml
local:
  name     : 'local'
  url     : 'http://127.0.0.1:8081/api/v1/servers/localhost/statistics'
  header   :
    X-API-Key: 'change_me'
```

---

# rabbitmq

Module monitor rabbitmq performance and health metrics.

Following charts are drawn:

1. **Queued Messages**
 * ready
 * unacknowledged

2. **Message Rates**
 * ack
 * redelivered
 * deliver
 * publish

3. **Global Counts**
 * channels
 * consumers
 * connections
 * queues
 * exchanges

4. **File Descriptors**
 * used descriptors

5. **Socket Descriptors**
 * used descriptors

6. **Erlang processes**
 * used processes
 
7. **Erlang run queue**
 * Erlang run queue

8. **Memory**
 * free memory in megabytes

9. **Disk Space**
 * free disk space in gigabytes

### configuration

```yaml
socket:
  name     : 'local'
  host     : '127.0.0.1'
  port     :  15672
  user     : 'guest'
  pass     : 'guest'

```

When no configuration file is found, module tries to connect to: `localhost:15672`.

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

# samba

Performance metrics of Samba file sharing.

It produces the following charts:

1. **Syscall R/Ws** in kilobytes/s
 * sendfile
 * recvfle

2. **Smb2 R/Ws** in kilobytes/s
 * readout
 * writein
 * readin
 * writeout

3. **Smb2 Create/Close** in operations/s
 * create
 * close

4. **Smb2 Info** in operations/s
 * getinfo
 * setinfo

5. **Smb2 Find** in operations/s
 * find

6. **Smb2 Notify** in operations/s
 * notify

7. **Smb2 Lesser Ops** as counters
 * tcon
 * negprot
 * tdis
 * cancel
 * logoff
 * flush
 * lock
 * keepalive
 * break
 * sessetup

### configuration

Requires that smbd has been compiled with profiling enabled.  Also required
that `smbd` was started either with the `-P 1` option or inside `smb.conf`
using `smbd profiling level`.

This plugin uses `smbstatus -P` which can only be executed by root.  It uses
sudo and assumes that it is configured such that the `netdata` user can
execute smbstatus as root without password.

For example:

    netdata ALL=(ALL)       NOPASSWD: /usr/bin/smbstatus -P

```yaml
update_every : 5 # update frequency
```

---

# sensors

System sensors information.

Charts are created dynamically.

### configuration

For detailed configuration information please read [`sensors.conf`](https://github.com/firehol/netdata/blob/master/conf.d/python.d/sensors.conf) file.

### possible issues

There have been reports from users that on certain servers, ACPI ring buffer errors are printed by the kernel (`dmesg`) when ACPI sensors are being accessed.
We are tracking such cases in issue [#827](https://github.com/firehol/netdata/issues/827).
Please join this discussion for help.

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

# smartd_log

Module monitor `smartd` log files to collect HDD/SSD S.M.A.R.T attributes.

It produces following charts (you can add additional attributes in the module configuration file):

1. **Read Error Rate** attribute 1

2. **Start/Stop Count** attribute 4

3. **Reallocated Sectors Count** attribute 5
 
4. **Seek Error Rate** attribute 7

5. **Power-On Hours Count** attribute 9

6. **Power Cycle Count** attribute 12

7. **Load/Unload Cycles** attribute 193

8. **Temperature** attribute 194

9. **Current Pending Sectors** attribute 197
 
10. **Off-Line Uncorrectable** attribute 198

11. **Write Error Rate** attribute 200
 
### configuration

```yaml
local:
  log_path : '/var/log/smartd/'
```

If no configuration is given, module will attempt to read log files in /var/log/smartd/ directory.
 
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

1. **Connections Statistics** in connections/s
 * accepted
 * dropped

2. **Client Requests** in requests/s
 * received

3. **All History Hit Rate Ratio** in percent
 * hit
 * miss
 * hitpass

4. **Current Poll Hit Rate Ratio** in percent
 * hit
 * miss
 * hitpass

5. **Expired Objects** in expired/s
 * objects
 
6. **Least Recently Used Nuked Objects** in nuked/s
 * objects


7. **Number Of Threads In All Pools** in threads
 * threads

8. **Threads Statistics** in threads/s
 * created
 * failed
 * limited
 
9. **Current Queue Length** in requests
 * in queue

10. **Backend Connections Statistics** in connections/s
 * successful
 * unhealthy
 * reused
 * closed
 * resycled
 * failed
 
10. **Requests To The Backend** in requests/s
 * received
 
11. **ESI Statistics** in problems/s
 * errors
 * warnings
 
12. **Memory Usage** in MB
 * free
 * allocated
 
13. **Uptime** in seconds
 * uptime
 
 
### configuration

No configuration is needed.

---

# web_log

Tails the apache/nginx/lighttpd/gunicorn log files to collect real-time web-server statistics.

It produces following charts:

1. **Response by type** requests/s
 * success (1xx, 2xx, 304)
 * error (5xx)
 * redirect (3xx except 304)
 * bad (4xx)
 * other (all other responses)

2. **Response by code family** requests/s
 * 1xx (informational)
 * 2xx (successful)
 * 3xx (redirect)
 * 4xx (bad)
 * 5xx (internal server errors)
 * other (non-standart responses)
 * unmatched (the lines in the log file that are not matched)

3. **Detailed Response Codes** requests/s (number of responses for each response code family individually)
 
4. **Bandwidth** KB/s
 * received (bandwidth of requests)
 * send (bandwidth of responses)

5. **Timings** ms (request processing time)
 * min (bandwidth of requests)
 * max (bandwidth of responses)
 * average (bandwidth of responses)

6. **Request per url** requests/s (configured by user)

7. **Http Methods** requests/s (requests per http method)

8. **Http Versions** requests/s (requests per http version)

9. **IP protocols** requests/s (requests per ip protocol version)

10. **Curent Poll Unique Client IPs** unique ips/s (unique client IPs per data collection iteration)

11. **All Time Unique Client IPs** unique ips/s (unique client IPs since the last restart of netdata)

 
### configuration

```yaml
nginx_log:
  name  : 'nginx_log'
  path  : '/var/log/nginx/access.log'

apache_log:
  name  : 'apache_log'
  path  : '/var/log/apache/other_vhosts_access.log'
  categories:
      cacti : 'cacti.*'
      observium : 'observium'
```

Module has preconfigured jobs for nginx, apache and gunicorn on various distros.

--- 
