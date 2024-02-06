### Understand the alert

The `mysql_galera_cluster_state_crit` alert is triggered when the Galera node state is either `Undefined`, `Joining`, or `Error`. This indicates that there is an issue with a Galera node in your MySQL Galera Cluster.

### What is a MySQL Galera Cluster?

MySQL Galera Cluster is a synchronous, multi-master database cluster that provides high availability, no data loss, and scalability for your MySQL databases. It uses Galera replication library and MySQL server to achieve these goals.

### Troubleshoot the alert

To troubleshoot the MySQL Galera Cluster State Critical alert, follow these steps:

1. Inspect the MariaDB error log

   Check the MariaDB error log for any relevant error messages that can help identify the issue.

   ```
   sudo tail -f /var/log/mysql/error.log
   ```

2. Check the Galera node's status

   Connect to the problematic MySQL node and check the Galera node status by running the following query:

   ```
   SHOW STATUS LIKE 'wsrep_%';
   ```
   
   Take note of the value of `wsrep_local_state` and `wsrep_local_state_comment`.

3. Diagnose the issue

   - If `wsrep_local_state` is 0 (`Undefined`), it means the node is not part of any cluster.
   - If `wsrep_local_state` is 1 (`Joining`), it means the node is trying to connect or reconnect to the cluster.
   - If `wsrep_local_state` is 5 (`Error`), it means the node has encountered a consistency error.

4. Resolve the issue

   - For an `Undefined` state, check and fix the wsrep configuration settings and restart the node.
   - For a `Joining` state, ensure that the node can communicate with the other nodes in the cluster and make sure that the cluster's state is healthy. Then, retry joining the node to the cluster.
   - For an `Error` state, the node may need to be resynchronized with the cluster. Restart the mysqld process on the affected node, or you may need to perform a full state transfer to recover.

5. Monitor the cluster

   After resolving the issue, monitor the cluster to ensure that all nodes are healthy and remain in-sync.

