### Understand the alert

This alert indicates that VerneMQ, an MQTT broker, is experiencing issues with inter-node message delivery within a clustered environment. The Netdata Agent calculates the amount of traffic dropped during communication with cluster nodes in the last minute. If you receive this alert, it means that the outgoing cluster buffer is full and some messages cannot be delivered.

### What does dropped messages mean?

Dropped messages occur when the outgoing cluster buffer becomes full, and VerneMQ cannot deliver messages between its nodes. This can happen due to a remote node being down or unreachable, causing the buffer to fill up and preventing efficient message delivery.

### Troubleshoot the alert

1. Check the connectivity and status of cluster nodes

   Verify that all cluster nodes are up, running and reachable. Use `vmq-admin cluster show` to get an overview of the cluster nodes and their connectivity status.

   ```
   vmq-admin cluster show
   ```

2. Investigate logs for any errors or warnings

   Inspect the logs of the VerneMQ node(s) for any errors or warning messages. This can provide insight into any potential problems related to the cluster or network.

   ```
   sudo journalctl -u vernemq
   ```

3. Increase the buffer size

   If the issue persists, consider increasing the buffer size. Adjust the `outgoing_clustering_buffer_size` value in the `vernemq.conf` file.

   ```
   outgoing_clustering_buffer_size = <new_buffer_size>
   ```

   Replace `<new_buffer_size>` with a larger value, for example, doubling the current buffer size. After updating the configuration, restart the VerneMQ service to apply the changes.

   ```
   sudo systemctl restart vernemq
   ```

4. Monitor the dropped messages

   Continue to monitor the dropped messages using Netdata, and check if the issue is resolved after increasing the buffer size.

### Useful resources

1. [VerneMQ Documentation - Clustering](https://vernemq.com/docs/clustering/)
2. [VerneMQ Logging and Monitoring](https://docs.vernemq.com/monitoring-vernemq/logging)
3. [Managing VerneMQ Configuration](https://docs.vernemq.com/configuration/)