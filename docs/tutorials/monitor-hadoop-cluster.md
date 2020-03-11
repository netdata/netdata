<!--
---
title: "Monitor a Hadoop cluster with Netdata"
custom_edit_url: https://github.com/netdata/netdata/edit/master/docs/tutorials/monitor-hadoop-cluster.md
---
-->

# Monitor a Hadoop cluster with Netdata

Hadoop is an [Apache project](https://hadoop.apache.org/) is a framework for processing large sets of data across a
distributed cluster of systems.

And while Hadoop is designed to be a highly-available and fault-tolerant service, those who operate a Hadoop cluster
will want to monitor the health and performance of their [Hadoop Distributed File System
(HDFS)](https://hadoop.apache.org/docs/r1.2.1/hdfs_design.html) and [Zookeeper](https://zookeeper.apache.org/)
implementations.

Netdata comes with built-in and pre-configured support for monitoring both HDFS and Zookeeper.

This tutorial assumes you have a Hadoop cluster, with HDFS and Zookeeper, running already. If you don't, please follow
the [official Hadoop
instructions](http://hadoop.apache.org/docs/stable/hadoop-project-dist/hadoop-common/SingleCluster.html) or an
alternative, like the guide available from
[DigitalOcean](https://www.digitalocean.com/community/tutorials/how-to-install-hadoop-in-stand-alone-mode-on-ubuntu-18-04).

For more specifics on the collection modules used in this tutorial, read the respective pages in our documentation:

-   [HDFS](../../collectors/go.d.plugin/modules/hdfs/README.md)
-   [Zookeeper](../../collectors/go.d.plugin/modules/zookeeper/README.md)

## Set up your HDFS and Zookeeper installations

As with all data sources, Netdata can auto-detect HDFS and Zookeeper nodes if you installed them using the standard
installation procedure.

For Netdata to collect HDFS metrics, it needs to be able to access the node's `/jmx` endpoint. You can test whether an
JMX endpoint is accessible by using `curl HDFS-IP:PORT/jmx`. For a NameNode, you should see output similar to the
following:

```json
{
  "beans" : [ {
    "name" : "Hadoop:service=NameNode,name=JvmMetrics",
    "modelerType" : "JvmMetrics",
    "MemNonHeapUsedM" : 65.67851,
    "MemNonHeapCommittedM" : 67.3125,
    "MemNonHeapMaxM" : -1.0,
    "MemHeapUsedM" : 154.46341,
    "MemHeapCommittedM" : 215.0,
    "MemHeapMaxM" : 843.0,
    "MemMaxM" : 843.0,
    "GcCount" : 15,
    "GcTimeMillis" : 305,
    "GcNumWarnThresholdExceeded" : 0,
    "GcNumInfoThresholdExceeded" : 0,
    "GcTotalExtraSleepTime" : 92,
    "ThreadsNew" : 0,
    "ThreadsRunnable" : 6,
    "ThreadsBlocked" : 0,
    "ThreadsWaiting" : 7,
    "ThreadsTimedWaiting" : 34,
    "ThreadsTerminated" : 0,
    "LogFatal" : 0,
    "LogError" : 0,
    "LogWarn" : 2,
    "LogInfo" : 348
  }, 
  { ... }
  ]
}
```

The JSON result for a DataNode's `/jmx` endpoint is slightly different:

```json
{
  "beans" : [ {
    "name" : "Hadoop:service=DataNode,name=DataNodeActivity-dev-slave-01.dev.loc
al-9866",
    "modelerType" : "DataNodeActivity-dev-slave-01.dev.local-9866",
    "tag.SessionId" : null,
    "tag.Context" : "dfs",
    "tag.Hostname" : "dev-slave-01.dev.local",
    "BytesWritten" : 500960407,
    "TotalWriteTime" : 463,
    "BytesRead" : 80689178,
    "TotalReadTime" : 41203,
    "BlocksWritten" : 16,
    "BlocksRead" : 16,
    "BlocksReplicated" : 4,
    ...
  },
  { ... }
  ]
}
```

If Netdata can't access the `/jmx` endpoint for either a NameNode or DataNode, it will not be able to auto-detect and
collect metrics from your HDFS implementation.

Zookeeper auto-detection relies on an accessible client port and a whitelisted `mntr` command. For more details on
`mntr`, see Zookeeper's documentation on [cluster
options](https://zookeeper.apache.org/doc/current/zookeeperAdmin.html#sc_clusterOptions) and [Zookeeper
commands](https://zookeeper.apache.org/doc/current/zookeeperAdmin.html#sc_zkCommands).

## Configure the HDFS and Zookeeper modules

To configure Netdata's HDFS module, navigate to your Netdata directory (typically at `/etc/netdata/`) and use
`edit-config` to initialize and edit your HDFS configuration file.

```bash
cd /etc/netdata/
sudo ./edit-config go.d/hdfs.conf
```

At the bottom of the file, you will see two example jobs, both of which are commented out:

```yaml
# [ JOBS ]
#jobs:
#  - name: namenode
#    url: http://127.0.0.1:9870/jmx
#
#  - name: datanode
#    url: http://127.0.0.1:9864/jmx
```

Uncomment these lines and edit the `url` value(s) according to your setup. Now's the time to add any other configuration
details, which you can find inside of the `hdfs.conf` file itself. Most production implementations will require TLS
certificates.

The result for a simple HDFS setup, running entirely on `localhost` and without certificate authentication, might look
like this:

```yaml
# [ JOBS ]
jobs:
  - name: namenode
    url: http://127.0.0.1:9870/jmx

  - name: datanode
    url: http://127.0.0.1:9864/jmx
```

At this point, Netdata should be configured to collect metrics from your HDFS servers. Let's move on to Zookeeper.

Next, use `edit-config` again to initialize/edit your `zookeeper.conf` file.

```bash
cd /etc/netdata/
sudo ./edit-config go.d/zookeeper.conf
```

As with the `hdfs.conf` file, head to the bottom, uncomment the example jobs, and tweak the `address` values according
to your setup. Again, you may need to add additional configuration options, like TLS certificates.

```yaml
jobs:
  - name    : local
    address : 127.0.0.1:2181

  - name    : remote
    address : 203.0.113.10:2182
```

Finally, restart Netdata.

```sh
sudo service restart netdata
```

Upon restart, Netdata should recognize your HDFS/Zookeeper servers, enable the HDFS and Zookeeper modules, and begin
showing real-time metrics for both in your Netdata dashboard. 🎉

## Configuring HDFS and Zookeeper alarms

The Netdata community helped us create sane defaults for alarms related to both HDFS and Zookeeper. You may want to
investigate these to ensure they work well with your Hadoop implementation.

-   [HDFS alarms](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/hdfs.conf)
-   [Zookeeper alarms](https://raw.githubusercontent.com/netdata/netdata/master/health/health.d/zookeeper.conf)

You can also access/edit these files directly with `edit-config`:

```bash
sudo /etc/netdata/edit-config health.d/hdfs.conf
sudo /etc/netdata/edit-config health.d/zookeeper.conf
```

For more information about editing the defaults or writing new alarm entities, see our [health monitoring
documentation](../../health/README.md).

## What's next?

If you're having issues with Netdata auto-detecting your HDFS/Zookeeper servers, or want to help improve how Netdata
collects or presents metrics from these services, feel free to [file an
issue](https://github.com/netdata/netdata/issues/new?labels=bug%2C+needs+triage&template=bug_report.md).

-   Read up on the [HDFS configuration
    file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/hdfs.conf) to understand how to configure
    global options or per-job options, such as username/password, TLS certificates, timeouts, and more.
-   Read up on the [Zookeeper configuration
    file](https://github.com/netdata/go.d.plugin/blob/master/config/go.d/zookeeper.conf) to understand how to configure
    global options or per-job options, timeouts, TLS certificates, and more.

[![analytics](https://www.google-analytics.com/collect?v=1&aip=1&t=pageview&_s=1&ds=github&dr=https%3A%2F%2Fgithub.com%2Fnetdata%2Fnetdata&dl=https%3A%2F%2Fmy-netdata.io%2Fgithub%2Fdocs%2Ftutorials%2Fmonitor-hadoop-cluster&_u=MAC~&cid=5792dfd7-8dc4-476b-af31-da2fdb9f93d2&tid=UA-64295674-3)](<>)
