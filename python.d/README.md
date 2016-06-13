Every plugin supports changing its data collection frequency by setting `update_every` variable in its configuration file.

The following python.d plugins are supported:

# mysql

The plugin will monitor one or more mysql servers

It will produce the following charts (if data is available):

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
4. mysql socket (optional)
5. mysql host (ip or hostname)
3. mysql port (defaults to 3306)

Here is an example for 2 servers updating data every 10 seconds

```js
update_every = 10

config=[
    {
    	'name'     : 'local',
        'user'     : 'root',
        'password' : 'blablablabla',
        'socket'   : '/var/run/mysqld/mysqld.sock'
    },{
        'name'     : 'remote',
        'user'     : 'admin',
        'password' : 'bla',
        'host'     : 'example.org',
        'port'     : '9000'
    }]
```

If no configuration is given, the plugin will attempt to connect to mysql server via unix socket at `/var/run/mysqld/mysqld.sock` without password and username `root`

---

