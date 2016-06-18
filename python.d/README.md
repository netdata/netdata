# Disclaimer

**Python plugin support is experimental and implementation may change in the future**

Currently every plugin must be written in python3.
All third party libraries should be installed system-wide or in `python_modules` directory.
Module configurations are written in YAML and **pyYAML is required**.

Every configuration file must have one of two formats:
1. Configuration for only one job:
```yaml
update_every : 2 # update frequency
retries      : 1 # how many failures in update() is tolerated
priority     : 20000 # where it is shown on dashboard

other_var1   : bla  # variables passed to module
other_var2   : alb
```
2. Configuration for many jobs (ex. mysql):
```yaml
# module defaults:
update_every : 2
retries      : 1
priority     : 20000

local:  # job name
  update_every : 5 # job update frequency
  retries      : 2 # job retries
  other_var1   : some_val # module specific variable

other_job: 
  priority     : 5 # job position on dashboard
  retries      : 20 # job retries
  other_var2   : val # module specific variable
```

`update_every`, `retries`, and `priority` are always optional.
---

The following python.d plugins are supported:

# mysql

The plugin will monitor one or more mysql servers

**Requirements:**
 * python module [MySQLdb](https://github.com/PyMySQL/mysqlclient-python) (faster) or [PyMySQL](https://github.com/PyMySQL/PyMySQL) (slower)

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

1. a name, anything you like, but keep it short
2. username which have access to database (deafults to 'root')
3. password (defaults to none)
4. mysql my.cnf configuration file
5. mysql socket (optional)
6. mysql host (ip or hostname)
7. mysql port (defaults to 3306)

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

If no configuration is given, the plugin will attempt to connect to mysql server via unix socket at `/var/run/mysqld/mysqld.sock` without password and with username `root`

---

