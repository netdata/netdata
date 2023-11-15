### Understand the alert

This alert indicates that the usable storage space allocated for your CockroachDB is being highly utilized. If the percentage of used space exceeds 85%, the alert raises a warning, and if it exceeds 95%, the alert becomes critical. High storage utilization can lead to performance issues and potential data loss if not properly managed.

### Troubleshoot the alert

1. Check the current storage utilization

To understand the current utilization, you can use SQL commands to query the `crdb_internal.kv_store_status` table.

```sql
SELECT node_id, store_id, capacity, used, available
FROM crdb_internal.kv_store_status;
```

This query will provide information about the available and used storage capacity of each node in your CockroachDB cluster.

2. Identify tables and databases with high storage usage

Use the following command to list the top databases in terms of storage usage:

```sql
SELECT database_name, sum(data_size_int) as total_size
FROM crdb_internal.tables
WHERE database_name != 'crdb_internal'
GROUP BY database_name
ORDER BY total_size DESC
LIMIT 10;
```

Additionally, you can list the top tables in terms of storage usage:

```sql
SELECT database_name, table_name, data_size
FROM crdb_internal.tables
WHERE database_name != 'crdb_internal'
ORDER BY data_size_int DESC
LIMIT 10;
```

3. Optimize storage usage

Based on your findings from steps 1 and 2, consider the following actions:

- Delete unneeded data from tables with high storage usage.
- Apply data compression to reduce the overall storage consumption.
- Archive old data or move it to external storage.

4. Add more storage to the nodes

If necessary, increase the storage allocated to your CockroachDB cluster by adding more space to each node.

- To increase the usable storage capacity, modify the `--store` flag when restarting your CockroachDB nodes. Set the new size by replacing `<YOUR_PATH>` with the actual store path and `<SIZE>` with the desired new size:

  ```
  --store=path=<YOUR_PATH>,size=<SIZE>
  ```

5. Add more nodes to the cluster

If increasing the storage capacity of your existing nodes isn't enough, consider adding more nodes to your CockroachDB cluster. By adding more nodes, you can distribute storage more evenly and prevent single points of failure due to storage limitations.

Refer to the [CockroachDB documentation](https://www.cockroachlabs.com/docs/stable/start-a-node.html) on how to add a new node to a cluster.