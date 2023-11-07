# mysql_replication_lag

## Database | MySQL, MariaDB

This alert presents the number of seconds that the replica is behind the master.  
Receiving this means that the replication SQL thread is far behind processing the source binary log.
A constantly high value (or an increasing one) indicates that the replica is unable to handle events
from the source in a timely fashion.

This alert is raised into warning when the metric exceeds 10 seconds.  
If the number of seconds that the replica is behind the master exceeds 30 seconds then the alert is
raised into critical.


> In MySQL, replication involves the source database writing down every change made to the data
> held within one or more databases in a special file known as the binary log. Once the replica
> instance has been initialized, it creates two threaded processes. The first, called the IO
> thread, connects to the source MySQL instance and reads the binary log events line by line,
> and then copies them over to a local file on the replica’s server called the relay log. The
> second thread, called the SQL thread, reads events from the relay log and then applies them
> to the replica instance as fast as possible.
>
> Recent versions of MySQL support two methods for replicating data. The difference between these
> replication methods has to do with how replicas track which database events from the source
> they’ve already processed.<sup>[1](
> https://www.digitalocean.com/community/tutorials/how-to-set-up-replication-in-mysql) </sup>

For further information, please have a look at the _References and Sources_ section.

<details><summary>References and Sources</summary>

1. [Replication in MySQL](
   https://www.digitalocean.com/community/tutorials/how-to-set-up-replication-in-mysql)
2. [MySQL Replication Slave Lag](
   https://www.percona.com/blog/2014/05/02/how-to-identify-and-cure-mysql-replication-slave-lag/)
3. [log_slow_slave_statements](
   https://dev.mysql.com/doc/refman/8.0/en/replication-options-replica.html#sysvar_log_slow_slave_statements)
4. [long_query_time](
   https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_long_query_time)
5. [log_slow_verbosity](
   https://www.percona.com/doc/percona-server/5.1/diagnostics/slow_extended.html?id=percona-server:features:slow_extended_51&redirect=2#log_slow_verbosity)

</details>

### Troubleshooting Section

<details><summary>Query optimization and "log_slow_slave_statements"</summary>

To minimize slave `SQL_THREAD` lag, focus on query optimization. The following logs will help you identify the problem:
1. Enable [log_slow_slave_statements](
https://dev.mysql.com/doc/refman/8.0/en/replication-options-replica.html#sysvar_log_slow_slave_statements)
to see queries executed by slave that take more than [long_query_time](
https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_long_query_time). 
2. To get more information about query performance, set the configuration option [log_slow_verbosity](
https://www.percona.com/doc/percona-server/5.1/diagnostics/slow_extended.html?id=percona-server:features:slow_extended_51&redirect=2#log_slow_verbosity) to `full`.
 
You can also read the Percona blog for a nice write-up about[MySQL replication slave lag](
https://www.percona.com/blog/2014/05/02/how-to-identify-and-cure-mysql-replication-slave-lag/). 

</details>
