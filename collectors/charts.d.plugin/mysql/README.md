# mysql

> THIS MODULE IS OBSOLETE.
> USE [THE PYTHON ONE](../../python.d.plugin/mysql) - IT SUPPORTS MULTIPLE JOBS AND IT IS MORE EFFICIENT

The plugin will monitor one or more mysql servers

It will produce the following charts:

1.  **Bandwidth** in kbps

-   in
-   out

2.  **Queries** in queries/sec

-   queries
-   questions
-   slow queries

3.  **Operations** in operations/sec

-   opened tables
-   flush
-   commit
-   delete
-   prepare
-   read first
-   read key
-   read next
-   read prev
-   read random
-   read random next
-   rollback
-   save point
-   update
-   write

4.  **Table Locks** in locks/sec

-   immediate
-   waited

5.  **Select Issues** in issues/sec

-   full join
-   full range join
-   range
-   range check
-   scan

6.  **Sort Issues** in issues/sec

-   merge passes
-   range
-   scan

## configuration

You can configure many database servers, like this:

You can provide, per server, the following:

1.  a name, anything you like, but keep it short
2.  the mysql command to connect to the server
3.  the mysql command line options to be used for connecting to the server

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

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fcollectors%2Fcharts.d.plugin%2Fmysql%2FREADME&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
