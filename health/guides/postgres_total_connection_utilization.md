### Understand the alert

This alert monitors the total `connection utilization` of a PostgreSQL database. If you receive this alert, it means that your `PostgreSQL` database is experiencing a high demand for connections. This can lead to performance degradation and, in extreme cases, could potentially prevent new connections from being established.

### What does connection utilization mean?

`Connection utilization` refers to the percentage of `database connections` currently in use compared to the maximum number of connections allowed by the PostgreSQL server. A high connection utilization implies that the server is handling a large number of concurrent connections, and its resources may be strained, leading to decreased performance.

### Troubleshoot the alert

1. Check the current connections to the PostgreSQL database:

   You can use the following SQL query to check the number of active connections for each database:
   
   ```
   SELECT datname, count(*) FROM pg_stat_activity GROUP BY datname;
   ```
   
   or use the following command to check the total connections to all databases:
   
   ```
   SELECT count(*) FROM pg_stat_activity;
   ```

2. Identify the source of increased connections:

   To find out which user or application is responsible for the high connection count, you can use the following SQL query:

   ```
   SELECT usename, application_name, count(*) FROM pg_stat_activity GROUP BY usename, application_name;
   ```

   This query shows the number of connections per user and application, which can help you identify the source of the increased connection demand.

3. Optimize connection pooling:

   If you are using an application server, such as `pgBouncer`, that supports connection pooling, consider adjusting the connection pool settings to better manage the available connections. This can help mitigate high connection utilization.

4. Increase the maximum connections limit:

   If your server has the necessary resources, you may consider increasing the maximum number of connections allowed by the PostgreSQL server. To do this, modify the `max_connections` configuration parameter in the `postgresql.conf` file and then restart the PostgreSQL service.

### Useful resources

1. [PostgreSQL: max_connections](https://www.postgresql.org/docs/current/runtime-config-connection.html#GUC-MAX-CONNECTIONS)
