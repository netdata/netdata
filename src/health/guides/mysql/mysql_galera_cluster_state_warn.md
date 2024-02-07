### Understand the alert

This alert checks the state of a Galera node in a MySQL Galera cluster. If you receive this alert, it means that the node is either in the **Donor/Desynced** state or the **Joined** state, which can indicate potential issues within the cluster.

### What does Donor/Desynced and Joined state mean?

1. **Donor/Desynced**: When a node is in the Donor/Desynced state, it is providing a State Snapshot Transfer (SST) to another node in the cluster. During this time, the node is not synchronized with the rest of the cluster and cannot process any write or commit requests.

2. **Joined**: In the Joined state, a node has completed the initial SST and is now catching up with any missing transactions through an Incremental State Transfer (IST).

### Troubleshoot the alert

1. Check the Galera cluster status with the following command:

   ```
   SHOW STATUS LIKE 'wsrep_%';
   ```

2. Verify if any node is in the Donor/Desynced or Joined state:

   ```
   SELECT VARIABLE_NAME, VARIABLE_VALUE FROM information_schema.GLOBAL_STATUS WHERE VARIABLE_NAME IN ('wsrep_local_state_comment', 'wsrep_cluster_status', 'wsrep_ready');
   ```

3. Identify the cause of the node state change. Some possible reasons are:

   - A new node has joined the cluster and requires an SST.
   - A node has been restarted, and it is rejoining the cluster.
   - A node experienced a temporary network issue and is now resynchronizing with the cluster.
   
4. Monitor the progress of the resynchronization process using the `SHOW STATUS` command, as provided above, and wait for the node to reach the *Synced* state.

5. If the node remains in the Donor/Desynced or Joined state for an extended period, investigate further to determine the cause of the issue:

   - Inspect the MySQL error log for any relevant messages.
   - Check for network issues or connectivity problems between the nodes.
   - Verify the cluster configuration and ensure all nodes have a consistent configuration.

6. Contact your DBA for assistance if the issue persists, as they may need to perform additional investigation and troubleshooting.

### Useful resources

1. [Galera Cluster's Documentation](https://galeracluster.com/library/documentation/)
