> THIS MODULE IS OBSOLETE.
> USE THE PYTHON ONE - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

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
