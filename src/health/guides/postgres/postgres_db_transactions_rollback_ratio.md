### Understand the alert

This alert calculates the `PostgreSQL database transactions rollback ratio` for the last five minutes. If you receive this alert, it means that the percentage of `aborted transactions` in the specified PostgreSQL database is higher than the defined threshold.

### What does transactions rollback ratio mean?

In a PostgreSQL database, the transactions rollback ratio represents the proportion of aborted transactions (those that roll back) in relation to the total number of transactions processed. A high rollback ratio may indicate issues with the application logic, database performance or excessive `deadlocks` causing transactions to be aborted frequently.

### Troubleshoot the alert

1. Check the PostgreSQL logs for any error messages or unusual activities related to transactions that might help identify the cause of the high rollback ratio.

   ```
   vi /var/log/postgresql/postgresql.log
   ```

   Replace `/var/log/postgresql/postgresql.log` with the appropriate path to your PostgreSQL log file.

2. Investigate recent database changes or application code modifications that might have led to the increased rollback ratio.

3. Examine the PostgreSQL database table and index statistics to identify potential performance bottlenecks.

   ```
   SELECT relname, seq_scan, idx_scan, n_tup_ins, n_tup_upd, n_tup_del, n_tup_hot_upd, n_live_tup, n_dead_tup, last_vacuum, last_analyze
   FROM pg_stat_all_tables
   WHERE schemaname = 'your_schema_name';
   ```

   Replace `your_schema_name` with the appropriate schema name.

4. Identify the most frequent queries that cause transaction rollbacks using pg_stat_statements view:

   ```
   SELECT substring(query, 1, 50) as short_query, calls, total_time, rows, 100.0 * shared_blks_hit/nullif(shared_blks_hit + shared_blks_read, 0) AS hit_percent
   FROM pg_stat_statements
   WHERE calls > 50
   ORDER BY (total_time / calls) DESC;
   ```

5. Investigate database locks and deadlocks using pg_locks:

   ```
   SELECT database, relation::regclass, mode, transactionid AS tid, virtualtransaction AS vtid, pid, granted
   FROM pg_catalog.pg_locks;
   ```

6. Make necessary changes in the application logic or database configuration to resolve the issues causing a high rollback ratio. Consult a PostgreSQL expert, if needed.

### Useful resources

1. [Monitoring PostgreSQL - rollback ratio](https://www.postgresql.org/docs/current/monitoring-stats.html#MONITORING-STATS-VIEWS)
2. [PostgreSQL: Database Indexes](https://www.postgresql.org/docs/current/indexes.html)
3. [PostgreSQL: Deadlocks](https://www.postgresql.org/docs/current/explicit-locking.html#LOCK-BUILT-IN-DEADLOCK-AVOIDANCE)
4. [PostgreSQL: Log files](https://www.postgresql.org/docs/current/runtime-config-logging.html)
5. [PostgreSQL: pg_stat_statements module](https://www.postgresql.org/docs/current/pgstatstatements.html)