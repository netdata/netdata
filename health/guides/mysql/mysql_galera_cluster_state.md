# mysql_galera_cluster_state

## Database | MySQL, MariaDB

This alert presents the state of a node in the Galera cluster. If you receive this, it could be an
indication that the node lost its connection to the Primary Component due to network partition.

The alert gets raised into warning if the metric has one of the values:

| Code | Description                                                                     | Alert Status |
|:----:|:--------------------------------------------------------------------------------|:------------:|
| `0`  | Undefined - indicates a starting node that is not part of the Primary Component |   Critical   |
| `1`  | Joining (requesting/receiving State Transfer) - node is joining the cluster     |   Critical   |
| `2`  | Donor/Desynced - node is the donor to the node joining the cluster              |   Warning    |
| `3`  | Joined - node has joined the cluster                                            |   Warning    |
| `>5` | -                                                                               |   Critical   |

For further information, please have a look at the *References and Sources* section.

<details><summary>References and Sources</summary>

1. [Galera Cluster Glossary](https://galeracluster.com/library/documentation/glossary.html)
2. [Wsrep status index](
   https://www.percona.com/doc/percona-xtradb-cluster/5.5/wsrep-status-index.html)
3. [Galera Cluster Notification command](
   https://galeracluster.com/library/documentation/notification-cmd.html)
</details>
