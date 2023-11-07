# cockroachdb_used_storage_capacity

## Database | CockroachDB

This alert presents the percentage of used storage space.  
If you receive this alert, then there is high storage capacity utilization from CockroachDB.

- This alert is raised to warning when the metric exceeds 85%.
- If the percentage of used storage space exceeds 95%, then the alert is raised to critical.

Definition of "size" on CockroachDB:

> The maximum size allocated to the node. When this size is reached, CockroachDB attempts to
> rebalance data to other nodes with available capacity. When there's no capacity elsewhere,
> this limit will be exceeded. Also, data may be written to the node faster than the cluster
> can rebalance it away; in this case, as long as capacity is available elsewhere, CockroachDB
> will gradually rebalance data down to the store limit.<sup>[1](
> https://www.cockroachlabs.com/docs/v21.2/cockroach-start#store) </sup>


<details><summary>References and Sources</summary>

1. [CockroachDB Size](https://www.cockroachlabs.com/docs/v21.2/cockroach-start#store) 
2. [CockroachDB Docs](https://www.cockroachlabs.com/docs/stable/ui-storage-dashboard.html)
</details>

### Troubleshooting Section

<details><summary>Increase the space available for CockroachDB data</summary>

If you had previously set a limit, then you can use the option `--store=path<YOUR PATH>,size=<SIZE>`
to increase the amount of available space.  
Make sure to replace the "YOUR PATH" with the actual store path and "SIZE" with the new size you want to set CockroachDB to.

> Note: If you haven't set a limit on the size, then the entire drive's size will be used. In this 
> case, you will see that the drive is full. Clearing some space or upgrading to a drive with a
> larger capacity are potential solutions.

</details>
