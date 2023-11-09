### Understand the alert

This alert indicates the current status of the Galera node cluster component in your MySQL or MariaDB database. Receiving this alert means that there is a potential issue with the cluster, such as a network partition that has caused the cluster to split into multiple components.

### Troubleshoot the alert

1. **Check the status of the Galera cluster**

   First, you need to determine the current status of the cluster to understand the severity of the issue. Check the value of the alert. Refer to the table in the given alert description to see which state your cluster is in.

2. **Verify cluster connectivity**

   If your cluster is in a non-primary state or disconnected, you should verify if all the nodes in your cluster can communicate with each other. You can use tools like `ping`, `traceroute`, or `mtr` to test connectivity between the cluster nodes. If there is a network issue, get in touch with your network administrator to resolve it.

3. **Examine node logs**

   Check the logs on each node for any indication of issues or error messages that can help identify the root cause of the problem. The logs are usually located in the `/var/log/mysqld.log` file or in the `/var/log/mysql/error.log` file. Look for lines that contain "ERROR" or "WARNING" as a starting point.

4. **Inspect Galera cluster settings**

   Analyze your Galera cluster configuration file (`/etc/my.cnf` or `/etc/mysql/my.cnf`) to make sure you have the correct settings, including the initial `wsrep_cluster_address` value, which defines the initial list of nodes in the cluster. If you find any misconfiguration, correct it and restart your database service.

5. **Force a new primary component**

   If you have a split-brain scenario, where multiple parts of the cluster are claiming to be the primary component, you need to force a new primary component. To do this, you can use the `SET GLOBAL wsrep_provider_options='pc.bootstrap=YES';` statement on one of the nodes that has the most up-to-date data. This action will force that node to act as the new primary component.

### Prevention

To minimize the risks of cluster issues, ensure the following:

1. Use reliable and redundant network connections between nodes.
2. Configure Galera cluster settings correctly.
3. Regularly monitor the cluster status and review logs.
4. Use the latest stable version of the Galera cluster software.

### Useful resources

1. [MariaDB Galera Cluster Documentation](
   https://mariadb.com/kb/en/getting-started-with-mariadb-galera-cluster/)
