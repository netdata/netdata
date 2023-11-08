### Understand the alert

The `riakkv_1h_kv_put_mean_latency` alert calculates the average time (in milliseconds) between the reception of client `PUT` requests and the subsequent responses to the clients over the last hour in a Riak KV database. If you receive this alert, it means that your Riak KV database is experiencing higher than normal latency in processing `PUT` requests.

### What is Riak KV?

Riak KV is a distributed NoSQL key-value data store designed to provide high availability, fault tolerance, operational simplicity, and scalability. The primary access method is through `PUT`, `GET`, `DELETE`, and `LIST` operations on keys.

### What does `PUT` latency mean?

`PUT` latency refers to the time it takes for the system to process a `PUT` request - from the moment the server receives the request until it sends a response back to the client. High `PUT` latency can impact the performance and responsiveness of applications relying on the Riak KV database.

### Troubleshoot the alert

- Check the Riak KV cluster health

  Use the `riak-admin cluster status` command to get an overview of the Riak KV cluster's health. Make sure there are no unreachable or down nodes in the cluster.

- Verify the Riak KV node performance

  Use the `riak-admin status` command to display various statistics of the Riak KV nodes. Pay attention to the `node_put_fsm_time_mean` and `node_put_fsm_time_95` metrics, as they are related to `PUT` latency.

- Inspect network conditions

  Use networking tools (e.g., `ping`, `traceroute`, `mtr`, `iftop`) to check for potential network latency issues between clients and the Riak KV servers.

- Evaluate the workload

  If the client application is heavily write-intensive, consider optimizing it to reduce the number of write operations or increase the capacity of the Riak KV cluster to handle the load.

- Review Riak KV logs

  Examine the Riak KV logs (`/var/log/riak/riak_kv.log` by default) for any error messages or unusual patterns that might be related to the increased `PUT` latency.

### Useful resources

1. [Riak KV Official Documentation](https://riak.com/docs/)
