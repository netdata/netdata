### Understand the alert

This alert monitors the percentage of transaction ID (TXID) exhaustion in a PostgreSQL database, specifically the rate at which the system is approaching a `TXID wraparound`. If the alert is triggered, it means that your PostgreSQL database is more than 90% towards exhausting its available transaction IDs, and you should take action to prevent transaction ID wraparound.

### What is TXID wraparound?

In PostgreSQL, transaction IDs are 32-bit integers, and a new one is assigned to each new transaction. Once the system has used all possible 32-bit integers for transaction IDs, it wraps back around to the beginning, reusing previous transaction IDs. This wraparound can lead to data loss or database unavailability if transactions' tuple visibility information becomes muddled.

### Troubleshoot the alert

1. Check the number of remaining transactions before wraparound. Connect to your PostgreSQL database, and run the following SQL query:

   ```sql
   SELECT datname, age(datfrozenxid) as age, current_limit FROM pg_database JOIN (SELECT setting AS current_limit FROM pg_settings WHERE name = 'autovacuum_vacuum_scale_factor') AS t1 ORDER BY age DESC;
   ```

2. Vacuum the database to prevent transaction ID wraparound. Run the following command:

   ```
   vacuumdb --all --freeze
   ```

   The command `vacuumdb` reclaims storage, optimizes the database for better performance, and prevents transaction ID wraparound.

3. Configure Autovacuum settings for long-term prevention. Adjust `autovacuum_vacuum_scale_factor`, `autovacuum_analyze_scale_factor`, `vacuum_cost_limit`, and `maintenance_work_mem` in the PostgreSQL configuration file `postgresql.conf`. Then, restart the PostgreSQL service for the changes to take effect.

   ```
   service postgresql restart
   ```

### Useful resources

1. [Preventing Transaction ID Wraparound Failures](https://www.postgresql.org/docs/current/routine-vacuuming.html#VACUUM-FOR-WRAPAROUND)
