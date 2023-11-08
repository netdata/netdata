### Understand the alert

This alert calculates the percentage of used space capacity across all DataNodes in the Hadoop Distributed File System (HDFS). If you receive this alert, it means that your HDFS DataNodes space capacity utilization is high.

The alert is triggered into warning when the percentage of used space capacity across all DataNodes is between 70-80% and in critical when it is between 80-90%.

### Troubleshoot the alert

Data is priceless. Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.

#### Check your Disk Usage across the cluster

1. Inspect the Disk Usage for each DataNode:

   ```
   root@netdata #  hadoop dfsadmin -report
   ```

   If all the DataNodes are in Disk pressure, you should consider adding more disk space. Otherwise, you can perform a balance of data between the DataNodes.

2. Perform a balance:

   ```
   root@netdata # hdfs balancer –threshold 15
   ```

   This means that the balancer will balance data by moving blocks from over-utilized to under-utilized nodes, until each DataNode’s disk usage differs by no more than plus or minus 15 percent.

#### Investigate high disk usage

1. Review your Hadoop applications, jobs, and scripts that write data to HDFS. Identify the ones with excessive disk usage or logging.

2. Optimize or refactor these applications, jobs, or scripts to reduce their disk usage.

3. Delete any unnecessary or temporary files from HDFS, if safe to do so.

4. Consider data compression or deduplication strategies, if applicable, to reduce storage usage in HDFS.

### Useful resources

1. [Apache Hadoop on Wikipedia](https://en.wikipedia.org/wiki/Apache_Hadoop)
2. [HDFS architecture](https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html)