### Understand the alert

This alert calculates the number of deadlocks in your PostgreSQL database in the last minute. If you receive this alert, it means that the number of deadlocks has surpassed the warning threshold (10 deadlocks per minute by default).

### What are deadlocks?

In a PostgreSQL database, a deadlock occurs when two or more transactions are waiting for one another to release a lock, causing a cyclical dependency. As a result, none of these transactions can proceed, and the database server may be unable to process other requests.

### Troubleshoot the alert

- Identify deadlock occurrences and problematic queries

1. Check the PostgreSQL log for deadlock occurrence messages. You can typically find these logs in `/var/log/postgresql/` or `/pg_log/`.
   
   Look for messages like: `DETAIL: Process 12345 waits for ShareLock on transaction 67890; blocked by process 98765.`

2. To find the problematic queries, examine the log entries before the deadlock messages. Most often, these entries will contain the SQL queries that led to the deadlocks.

- Analyze and optimize the problematic queries

1. Analyze the execution plans of the problematic queries using the `EXPLAIN` command. This can help you identify which parts of the query are causing the deadlock.

2. Optimize the queries by rewriting them or by adding appropriate indices to speed up the processing time.

- Avoid long-running transactions

1. Long-running transactions increase the chances of deadlocks. Monitor your database for long-running transactions and try to minimize their occurrence.

2. Set sensible lock timeouts to avoid transactions waiting indefinitely for a lock.

- Review your application logic

1. Inspect your application code for any circular dependencies that could lead to deadlocks.

2. Use advisory locks when possible to minimize lock contention in the database.

### Useful resources

1. [PostgreSQL: Deadlocks](https://www.postgresql.org/docs/current/explicit-locking.html#LOCKING-DEADLOCKS)
