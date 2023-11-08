### Understand the alert

This alert monitors the number of missing blocks in a Hadoop Distributed File System (HDFS). If you receive this alert, it means that there is at least one missing block in one of the DataNodes. This issue could be caused by a problem with the underlying storage or filesystem of a DataNode.

### Troubleshooting the alert

#### Fix corrupted or missing blocks

Before you perform any action, make sure that you have taken any necessary backup steps. Netdata is not liable for any loss or corruption of any data, database, or software.

1. Identify which files are facing issues.

```sh
root@netdata # hdfs fsck -list-corruptfileblocks
```

Inspect the output and track the path(s) to the corrupted files.

2. Determine where the file's blocks might live. If the file is larger than your block size, it consists of multiple blocks.

```sh
root@netdata # hdfs fsck <path_to_corrupted_file> -locations -blocks -files
```

This command will print out locations for every "problematic" block.

3. Search in the corresponding DataNode and the NameNode's logs for the machine or machines on which the blocks lived. Try looking for filesystem errors on those machines. Use `fsck`.

4. If there are files or blocks that you cannot fix, you must delete them so that the HDFS becomes healthy again.

- For a specific file:

```sh
root@netdata # hdfs fs -rm <path_to_file_with_unrecoverable_blocks>
```

- For all the "problematic" files:

```sh
hdfs fsck / -delete
```

### Useful resources

1. [Apache Hadoop on Wikipedia](https://en.wikipedia.org/wiki/Apache_Hadoop)
2. [HDFS Architecture](https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html)
3. [Man Pages of fsck](https://linux.die.net/man/8/fsck)