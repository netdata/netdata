### Understand the alert

The `postgres_table_bloat_size_perc` alert measures the bloat size percentage in a PostgreSQL database table. If you receive this alert, it means that the bloat size in a particular table in your PostgreSQL database has crossed the warning or critical threshold.

### What is bloat size?

In PostgreSQL, bloat size refers to the wasted storage space caused by dead rows and unused space that accumulates in database tables over time. It is a result of frequent database operations (inserts, updates, and deletes), impacting database performance and storage footprint.

### Troubleshoot the alert

- Investigate the bloat size and impacted table

To get a detailed report on bloated tables in your PostgreSQL database, use the [`pgstattuple`](https://www.postgresql.org/docs/current/pgstattuple.html) extension. First, install the extension if it isn't already installed:

   ```
   CREATE EXTENSION pgstattuple;
   ```

Then, run the following query to find the bloated tables:

   ```sql
   SELECT 
      schemaname, tablename, 
      pg_size_pretty(bloat_size) AS bloat_size,
      round(bloat_ratio::numeric, 2) AS bloat_ratio
   FROM (
      SELECT 
         schemaname, tablename,
         bloat_size, table_size, (bloat_size / table_size) * 100 as bloat_ratio 
      FROM pgstattuple.schema_bloat
   ) sub_query
   WHERE bloat_ratio > 10
   ORDER BY bloat_ratio DESC;
   ```

- Reclaim storage space

Reducing the bloat size in PostgreSQL tables involves reclaiming wasted storage space. Here are two approaches:

  1. **VACUUM**: The `VACUUM` command helps clean up dead rows and compact the space used by the table. Use the following command to clean up the impacted table:

      ```
      VACUUM VERBOSE ANALYZE <schema_name>.<table_name>;
      ```

  2. **REINDEX**: If the issue persists after using `VACUUM`, consider REINDEXing the table. This command rebuilds the table's indexes, which can improve query performance and reduce bloat. It can be more intrusive than `VACUUM`, be sure you understand its implications before running:

      ```
      REINDEX TABLE <schema_name>.<table_name>;
      ```

- Monitor the bloat size

Continue monitoring the bloat size in your PostgreSQL tables by regularly checking the `postgres_table_bloat_size_perc` alert on Netdata.

### Useful resources

1. [How to monitor and fix Database bloats in PostgreSQL?](https://blog.netdata.cloud/postgresql-database-bloat/)
