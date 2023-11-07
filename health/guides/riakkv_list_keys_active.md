### Understand the alert

This alert indicates that currently there are active `list keys` operations in Finite State Machines (FSM) on your Riak KV database. Running `list keys` in Riak is a resource-intensive operation and can significantly affect the performance of the cluster, and it is not recommended for production use.

### What are list keys operations in Riak?

`List keys` operations in Riak involve iterating through all keys in a bucket to return a list of keys. The reason this is expensive in terms of resources is that Riak needs to traverse the entire dataset to generate a list of keys. As the dataset grows, the operation consumes more resources and takes longer to process the list, which can lead to reduced performance and scalability.

### Troubleshoot the alert

To address the `riakkv_list_keys_active` alert, follow these steps:

1. Identify the processes and applications running `list keys` operations:

   Monitor your application logs and identify the processes or applications that are using these operations. You may need to enable additional logging to capture information related to `list keys`.

2. Evaluate the necessity of `list keys` operations:

   Work with your development team and determine if there's a specific reason these operations are being used. If they are not necessary, consider replacing them with other, more efficient data retrieval techniques.

3. Optimize data retrieval:

   If it is necessary to retrieve keys in your application, consider using an alternative strategy such as Secondary Indexes (2i) or implementing a custom solution tailored to your specific use case.

4. Monitor the system:

   After making changes to your application, continue monitoring the active list key FSMs using Netdata to ensure that the number of active list keys operations is reduced.

### Useful resources

1. [Riak KV Operations](https://docs.riak.com/riak/kv/latest/developing/usage/operations/index.html)
