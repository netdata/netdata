# mysql_replication

## Database | MySQL, MariaDB

This alert monitors the replication status of the MySQL server.  
If you receive this, either both or one of the I/O and SQL threads are not running.

This alert is raised into critical if replication has stopped.


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
2. [MySQL documentation](
   https://dev.mysql.com/doc/refman/5.7/en/replication-administration-status.html)
3. [Section 8.14.6, “Replication Replica I/O
   Thread States”](https://dev.mysql.com/doc/refman/5.7/en/replica-io-thread-states.html)
4. [Section 8.14.7, “Replication Replica SQL Thread
   States”](https://dev.mysql.com/doc/refman/5.7/en/replica-sql-thread-states.html)
</details>

### Troubleshooting Section

<details><summary>Check which thread is not running</summary>

From the MySQL command line you can run:

- For MySQL and MariaDB before v10.2.0: 
   
  ```
  SHOW SLAVE STATUS\G
  ```
- For MariaDB v10.2.0+:
  
  ```
  SHOW ALL SLAVES STATUS\G
  ```
   
This will show you three important rows among other info:

> - Slave_IO_State:  
    The current status of the replica. See [Section 8.14.6, “Replication Replica I/O
    Thread States”](https://dev.mysql.com/doc/refman/5.7/en/replica-io-thread-states.html), and
    [Section 8.14.7, “Replication Replica SQL Thread 
    States”](https://dev.mysql.com/doc/refman/5.7/en/replica-sql-thread-states.html), for more
    information.
>
>
> - Slave_IO_Running:  
    Whether the I/O thread for reading the source's binary log is running.
    Normally, you want this to be **Yes** unless you have not yet started replication or have
    explicitly stopped it with STOP SLAVE.
>
>
> - Slave_SQL_Running:  
    Whether the SQL thread for executing events in the relay log is running. As
    with the I/O thread, this should normally be **Yes**.<sup> [2](
    https://dev.mysql.com/doc/refman/5.7/en/replication-administration-status.html) </sup>

For more info you can refer to the [MySQL documentation](
https://dev.mysql.com/doc/refman/5.7/en/replication-administration-status.html).
</details>
