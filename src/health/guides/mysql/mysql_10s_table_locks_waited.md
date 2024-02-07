### Understand the alert

This alert is triggered when there's a high number of `table locks waited` in the last 10 seconds for a MySQL database. Table locks prevent multiple processes from writing to a table at the same time, ensuring the integrity of the data. However, too many table locks waiting can indicate a performance issue, as it could mean that some queries are causing deadlocks or taking too long to complete.

### Troubleshoot the alert

1. Identify queries causing locks

   Use the following MySQL command to view the currently running queries and identify the ones causing the table locks:

   ```
   SHOW FULL PROCESSLIST;
   ```

2. Examine locked tables

   Use the following command to find more information about the locked tables:

   ```
   SHOW OPEN TABLES WHERE In_use > 0;
   ```

3. Optimize query performance

   Analyze the queries causing the table locks and optimize them to improve performance. This may include creating or modifying indexes, optimizing the SQL query structure, or adjusting the MySQL server configuration settings.

4. Consider using InnoDB

   If your MySQL database is using MyISAM storage engine, consider switching to InnoDB storage engine to take advantage of row-level locking and reduce the number of table locks.

5. Monitor MySQL performance

   Keep an eye on MySQL performance metrics such as table locks, query response times, and overall database performance to prevent future issues. Tools like the Netdata Agent can help in monitoring MySQL performance.

### Useful resources

1. [InnoDB Locking and Transaction Model](https://dev.mysql.com/doc/refman/8.0/en/innodb-locking-transaction-model.html)
