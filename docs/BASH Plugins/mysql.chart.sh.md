Documentation is not written yet. This is just a quick and dirty guide.

The shell script code is [here](https://github.com/firehol/netdata/blob/master/charts.d/mysql.chart.sh)

This collector supports any number of mysql servers.
It generates 20+ charts for each mysql server.

By default (not configured) it will attempt to connect to a mysql server at localhost. If it succeeds it will proceed.

You can configure the mysql servers to connect to, by editing `/etc/netdata/mysql.conf`.

To setup 2 servers (MY_A and MY_B) this is what you will need:

```sh

# you can define a different mysql client per server.
# if you want to use the mysql command from the system path,
# you can omit setting it.
mysql_cmds[MY_A]="/path/to/mysql"
mysql_cmds[MY_B]="/path/to/mysql"

# mysql client command line options to connect to the server
mysql_opts[MY_A]="-h serverA -u user"
mysql_opts[MY_B]="--defaults-file=/etc/mysql/serverB.cnf"

```

Any number of servers can be configured.

keep in mind that local user `netdata` should be able to access your mysql to execute this:

```sh
$mysql $OPTIONS -s -e "show global status;"
```

where:

 - `$mysql` is `mysql_cmds[X]`
 - `$OPTIONS` is `mysql_opts[X]`


It might also be good to add `-u user` to `mysql_opts[X]` to force the user you have given access rights to execute `show global status` on your mysql server.

Since this data collector is written in shell scripting, the connection to the mysql server is initiated and closed on each iteration.

It is in our TODOs to re-write it in node.js which will provide a more efficient way of collecting mysql performance counters.

