### Understand the alert

The `riakkv_kv_put_slow` alert is triggered when the average processing time for PUT requests in Riak KV database increases significantly in comparison to the last hour's average, suggesting that the server is overloaded.

### What does server overloaded mean?

An overloaded server means that the server is unable to handle the incoming requests efficiently, leading to increased processing times and degraded performance. Sometimes, it might result in request timeouts or even crashes.

### Troubleshoot the alert

To troubleshoot this alert, follow the below steps:

1. **Check current Riak KV performance**
   
   Use `riak-admin` tool's `status` command to check the current performance of the Riak KV node:
   
   ```
   riak-admin status
   ```

   Look for the following key performance indicators (KPIs) for PUT requests:
   - riak_kv.put_fsm.time.95 (95th percentile processing time for PUT requests)
   - riak_kv.put_fsm.time.99 (99th percentile processing time for PUT requests)
   - riak_kv.put_fsm.time.100 (Maximum processing time for PUT requests)
  
   If any of these values are significantly higher than their historical values, it may indicate an issue with the node's performance.

2. **Identify high-load operations**

   Examine the application logs or Riak KV logs for recent activity such as high volume of PUT requests, bulk updates or deletions, or other intensive database operations that could potentially cause the slowdown.

3. **Investigate other system performance indicators**

   Check the server's CPU, memory, and disk I/O usage to identify any resource constraints that could be affecting the performance of the Riak KV node.

4. **Review Riak KV configuration**

   Analyze the Riak KV configuration settings to ensure that they are optimized for your specific use case. Improperly configured settings can lead to performance issues.

5. **Consider scaling the Riak KV cluster**

   If the current Riak KV cluster is not able to handle the increasing workload, consider adding new nodes to the cluster to distribute the load and improve performance.

