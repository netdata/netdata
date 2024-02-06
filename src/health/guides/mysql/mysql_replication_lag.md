### Understand the alert

This alert presents the number of seconds that the replica is behind the master. Receiving this means that the replication SQL thread is far behind processing the source binary log. A constantly high value (or an increasing one) indicates that the replica is unable to handle events from the source in a timely fashion.

This alert is raised into warning when the metric exceeds 10 seconds. If the number of seconds that the replica is behind the master exceeds 30 seconds then the alert is raised into critical.


### Troubleshoot the alert

- Query optimization and "log_slow_slave_statements"

To minimize slave `SQL_THREAD` lag, focus on query optimization. The following logs will help you identify the problem:
1. Enable [log_slow_slave_statements](https://dev.mysql.com/doc/refman/8.0/en/replication-options-replica.html#sysvar_log_slow_slave_statements) to see queries executed by slave that take more than [long_query_time](https://dev.mysql.com/doc/refman/8.0/en/server-system-variables.html#sysvar_long_query_time). 
2. To get more information about query performance, set the configuration option [log_slow_verbosity](https://www.percona.com/doc/percona-server/5.1/diagnostics/slow_extended.html?id=percona-server:features:slow_extended_51&redirect=2#log_slow_verbosity) to `full`.
 
You can also read the Percona blog for a nice write-up about[MySQL replication slave lag](https://www.percona.com/blog/2014/05/02/how-to-identify-and-cure-mysql-replication-slave-lag/). 

### Useful resources

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

