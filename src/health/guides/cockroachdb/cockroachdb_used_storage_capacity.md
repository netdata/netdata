### Understand the Alert

This alert indicates high storage capacity utilization in CockroachDB.

### Definition of "size" on CockroachDB:

The maximum size allocated to the node. When this size is reached, CockroachDB attempts to rebalance data to other nodes with available capacity. When there's no capacity elsewhere, this limit will be exceeded. Also, data may be written to the node faster than the cluster can rebalance it away; in this case, as long as capacity is available elsewhere, CockroachDB will gradually rebalance data down to the store limit.

### Troubleshoot the Alert

- Increase the space available for CockroachDB data

If you had previously set a limit, then you can use the option `--store=path<YOUR PATH>,size=<SIZE>` to increase the amount of available space. Make sure to replace the "YOUR PATH" with the actual store path and "SIZE" with the new size you want to set CockroachDB to.

Note: If you haven't set a limit on the size, then the entire drive's size will be used. In this case, you will see that the drive is full. Clearing some space or upgrading to a drive with a larger capacity are potential solutions.

- Inspect the disk usage by tables and indexes

CockroachDB provides the `experimental_disk_usage` builtin SQL function that allows you to check the disk usage by tables and indexes within a given database. This can help you identify the main storage consumers in your cluster.

To run this command, first connect to your CockroachDB instance with `cockroach sql`, then execute the following query:

```sql
SELECT * FROM [SHOW experimental_disk_usage('<database_name>')];
```

Make sure to replace `<database_name>` with the actual name of the database you want to inspect. This will return a list of tables and indexes with their respective disk usage.

- Rebalance the cluster data to other nodes with available capacity

CockroachDB automatically rebalances data across nodes by default. If the data rebalancing is not happening fast enough, you can try to speed up this process by [adjusting `zone configurations`](https://www.cockroachlabs.com/docs/stable/configure-replication-zones.html) or by [increasing the default rebalancing rate](https://www.cockroachlabs.com/docs/stable/cluster-settings.html#kv_range_replication_rate_bytes_per_second).

- Purge old, unnecessary data

Inspect your data and consider purging old or unnecessary data from the database. Be cautious while performing this operation and double-check the data you intend to remove.

- Archive old data

If the data cannot be purged, consider archiving it in a more compact format or moving it to a separate database or storage system to reduce the storage usage on the affected CockroachDB node.


## Useful resources

1. [CockroachDB Size](https://www.cockroachlabs.com/docs/v21.2/cockroach-start#store)
2. [CockroachDB Docs](https://www.cockroachlabs.com/docs/stable/ui-storage-dashboard.html)

