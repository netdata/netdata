# mysql_galera_cluster_size

## Database | MySQL, MariaDB

This alert presents the current Galera cluster size, compared to the maximum size in the last 2
minutes.

If you receive this alert, then it may indicate a network connectivity problem or 
that MySQL is down on one node.

This alert is raised into warning if the current Galera cluster size is larger than the maximum 
size in the last 2 minutes.  

If the current Galera cluster size is less than the maximum size in the last sixty seconds, then the 
alert is escalated into critical.

<details><summary>References and Sources</summary>

1. [Galera Cluster Training Library](
   https://galeracluster.com/library/training/tutorials/galera-monitoring.html)

</details>

### Troubleshooting Section

<details><summary>Check Node Status</summary>

Refer to the [Galera Cluster training library](https://galeracluster.com/library/training/tutorials/galera-monitoring.html)
for documentation on cluster health monitoring.

</details>
