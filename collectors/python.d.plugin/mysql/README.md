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

1. username which have access to database (defaults to 'root')
2. password (defaults to none)
3. mysql my.cnf configuration file
4. mysql socket (optional)
5. mysql host (ip or hostname)
6. mysql port (defaults to 3306)

Here is an example for 3 servers:

```yaml
update_every : 10
priority     : 90100

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
```

If no configuration is given, module will attempt to connect to mysql server via unix socket at `/var/run/mysqld/mysqld.sock` without password and with username `root`

---

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fpython.d.plugin%2Fmysql%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)]()
