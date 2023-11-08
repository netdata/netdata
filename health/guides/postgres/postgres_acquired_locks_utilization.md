### Understand the alert

This alert monitors the average `acquired locks utilization` over the last minute in PostgreSQL databases. If you receive this alert, it means that the acquired locks utilization for your system is near or above the warning threshold (15% or 20%).

### What are acquired locks?

In PostgreSQL, a lock is a mechanism used to control access to shared resources, such as database tables or rows. When multiple users or tasks are working with the database, locks help coordinate their activities and prevent conflicts.

Acquired locks utilization refers to the percentage of locks currently in use in the system, compared to the total number of locks available.

### Troubleshoot the alert

1. Identify the most lock-intensive queries:

   You can use the following SQL query to get the list of most lock-intensive queries running on your PostgreSQL server:

   ```
   SELECT pid, locktype, mode, granted, client_addr, query_start, now() - query_start AS duration, query
   FROM pg_locks l
   JOIN pg_stat_activity a ON l.pid = a.pid
   WHERE query != '<IDLE>'
   ORDER BY duration DESC;
   ```

2. Analyze the problematic queries and look for ways to optimize them, such as:

   a. Adding missing indexes for faster query execution.
   b. Updating and optimizing query plans.
   c. Adjusting lock types or lock levels, if possible.

3. Check the overall health and performance of your PostgreSQL server:

   a. Monitor the CPU, memory, and disk usage.
   b. Consider configuring the autovacuum settings to maintain your database's health.
   
4. Monitor database server logs for any errors or issues.

5. If the problem persists, consider adjusting the warning threshold (`warn` option), or even increasing the available locks in the PostgreSQL configuration (`max_locks_per_transaction`).

### Useful resources

1. [PostgreSQL Locks Monitoring](https://www.postgresql.org/docs/current/monitoring-locks.html)
2. [PostgreSQL Server Activity statistics](https://www.postgresql.org/docs/current/monitoring-stats.html)
