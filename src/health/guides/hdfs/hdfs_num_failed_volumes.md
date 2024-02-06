### Understand the alert

This alert is triggered when the number of failed volumes in your Hadoop Distributed File System (HDFS) cluster increases. A failed volume may be due to hardware failure or misconfiguration, such as duplicate mounts. When a single volume fails on a DataNode, the entire node may go offline depending on the `dfs.datanode.failed.volumes.tolerated` setting for your cluster. This can lead to increased network traffic and potential performance degradation as the NameNode needs to copy any under-replicated blocks lost on that node.

### Troubleshoot the alert

#### 1. Identify which DataNode has a failing volume

Use the `dfsadmin -report` command to identify the DataNodes that are offline:

```bash
root@netdata # dfsadmin -report
```

Find any nodes that are not reported in the output of the command. If all nodes are listed, you'll need to run the next command for each DataNode.

#### 2. Review the volumes status

Use the `hdfs dfsadmin -getVolumeReport` command, specifying the DataNode hostname and port:

```bash
root@netdata # hdfs dfsadmin -getVolumeReport datanodehost:port
```

#### 3. Inspect the DataNode logs

Connect to the affected DataNode and check its logs using `journalctl -xe`. If you have the Netdata Agent running on the DataNodes, you should be able to identify the problem. You may also receive alerts about the disks and mounts on this system.

#### 4. Take necessary actions

Based on the information gathered in the previous steps, take appropriate actions to resolve the issue. This may include:

- Repairing or replacing faulty hardware.
- Fixing misconfigurations such as duplicate mounts.
- Ensuring that the HDFS processes are running on the affected DataNode.
- Ensuring that the affected DataNode is properly communicating with the NameNode.

**Note**: When working with HDFS, it's essential to have proper backups of your data. Netdata is not responsible for any loss or corruption of data, database, or software.

### Useful resources

1. [Apache Hadoop on Wikipedia](https://en.wikipedia.org/wiki/Apache_Hadoop)
2. [HDFS architecture](https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html)
3. [HDFS 3.3.1 commands guide](https://hadoop.apache.org/docs/current/hadoop-project-dist/hadoop-hdfs/HDFSCommands.html)
