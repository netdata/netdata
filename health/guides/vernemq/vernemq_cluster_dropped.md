# vernemq_cluster_dropped

**Messaging | VerneMQ**

VerneMQ is a MQTT publish/subscribe message broker which implements the OASIS industry standard MQTT
protocol.

The Netdata agent calculates the amount of traffic dropped during communication with the cluster
nodes in the last minute. This alert indicates that the outgoing cluster buffer is full.

Receiving this alert most likely means that a remote node is down or unreachable, but it could also
indicate that the VerneMQ is experiencing problems with inter-node message delivery. The
non-dispatched messages are queued in this buffer.

### Troubleshooting section:

<details>
<summary>Increase the cluster buffer </summary>

To make your cluster more tolerant to disconnections of nodes, you can increase the size of the
`outgoing_clustering_buffer_size` buffer.

1. Edit the VerneMQ configuration file. By default it is located under `/etc/vernemq` folder.

    ```
    root@netdata # vim /etc/vernemq/vernemq.conf 
    ```

2. Append the `outgoing_clustering_buffer_size` value, the default value is 10000 bytes. Try to
   increase it to 15000

    ```
    # vim /etc/vernemq/vernemq.conf
    . . .  
    outgoing_clustering_buffer_size = 15000
    . . .
    ```

3. Restart the VerneMQ service

   ```
   root@netdata # systemctl restart vernemq.service
   ```

4. Test with the same workload that triggered the alarm originally. If this alert still occurs, try
   to double this value and re-test.

5. In case the problem still exists, you must check for issues in the nodes that are unavailable.

</details>

