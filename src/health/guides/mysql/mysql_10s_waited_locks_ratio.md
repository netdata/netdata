### Understand the alert

This alert indicates a high ratio of waited table locks in your MySQL database over the last 10 seconds. If you receive this alert, it means that there might be performance issues due to contention for table locks.

### What are table locks?

Table locks are a method used by MySQL to ensure data consistency and prevent multiple clients from modifying the same data at the same time. When a client attempts to modify data, it must first acquire a lock on the table. If the lock is not available, the client must wait until the lock is released by another client.

### Troubleshoot the alert

1. Identify problematic queries:

   Use the following command to display the queries that are causing table locks in your MySQL database:

   ```
   SHOW FULL PROCESSLIST;
   ```

   Look for queries with a state of `'Locked'` or `'Waiting for table lock'` and note down their details.

2. Optimize your queries:

   Analyze the problematic queries identified in the previous step and try to optimize them. You can use `EXPLAIN` or other similar tools to get insights into the performance of the queries.

3. Consider splitting your table(s):

   If the problem persists after optimizing the queries, consider splitting the large tables into smaller ones. This can help to reduce contention for table locks and improve performance.

4. Use replication:

   Another solution to this issue is the implementation of MySQL replication, which can reduce contention for table locks by allowing read queries to be executed on replica servers rather than the primary server.

### Useful resources

1. [Documentation: Table Locking Issues](https://dev.mysql.com/doc/refman/5.7/en/table-locking.html)
2. [MySQL Replication](https://dev.mysql.com/doc/refman/8.0/en/replication.html)
