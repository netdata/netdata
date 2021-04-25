# -*- coding: utf-8 -*-
# Description: ClickHouse netdata python.d module
# Author: Federico Ceratto <federico@openobservatory.org>
# SPDX-License-Identifier: GPL-3.0-or-later

from bases.FrameworkServices.UrlService import UrlService

# Generate CHARTS and ORDER at start time
# category -> name -> description
_metrics_summary = {
    "metrics": (
        ("Query", "Number of executing queries"),
        ("Merge", "Number of executing background merges"),
        ("PartMutation", "Number of mutations (ALTER DELETE/UPDATE)"),
        ("ReplicatedFetch", "Number of data parts fetching from replica"),
        ("ReplicatedSend", "Number of data parts sending to replicas"),
        ("ReplicatedChecks", "Number of data parts checking for consistency"),
        (
            "BackgroundPoolTask",
            "Number of active tasks in BackgroundProcessingPool (merges, mutations, fetches or replication queue bookkeeping)",
        ),
        (
            "BackgroundSchedulePoolTask",
            "Number of active tasks in BackgroundSchedulePool. This pool is used for periodic tasks of ReplicatedMergeTree like cleaning old data parts, altering data parts, replica re-initialization, etc.",
        ),
        (
            "DiskSpaceReservedForMerge",
            "Disk space reserved for currently running background merges. It is slightly more than total size of currently merging parts.",
        ),
        (
            "DistributedSend",
            "Number of connections sending data, that was INSERTed to Distributed tables, to remote servers. Both synchronous and asynchronous mode.",
        ),
        (
            "QueryPreempted",
            "Number of queries that are stopped and waiting due to priority setting.",
        ),
        (
            "TCPConnection",
            "Number of connections to TCP server (clients with native interface)",
        ),
        ("HTTPConnection", "Number of connections to HTTP server"),
        (
            "InterserverConnection",
            "Number of connections from other replicas to fetch parts",
        ),
        ("OpenFileForRead", "Number of files open for reading"),
        ("OpenFileForWrite", "Number of files open for writing"),
        ("Read", "Number of read (read, pread, io_getevents, etc.) syscalls in fly"),
        (
            "Write",
            "Number of write (write, pwrite, io_getevents, etc.) syscalls in fly",
        ),
        (
            "SendExternalTables",
            "Number of connections that are sending data for external tables to remote servers. External tables are used to implement GLOBAL IN and GLOBAL JOIN operators with distributed subqueries.",
        ),
        ("QueryThread", "Number of query processing threads"),
        (
            "ReadonlyReplica",
            "Number of Replicated tables that are currently in readonly state due to re-initialization after ZooKeeper session loss or due to startup without ZooKeeper configured.",
        ),
        (
            "LeaderReplica",
            "Number of Replicated tables that are leaders. Leader replica is responsible for assigning merges, cleaning old blocks for deduplications and a few more bookkeeping tasks. There may be no more than one leader across all replicas at one moment of time. If there is no leader it will be elected soon or it indicate an issue.",
        ),
        (
            "MemoryTracking",
            "Total amount of memory (bytes) allocated in currently executing queries. Note that some memory allocations may not be accounted.",
        ),
        (
            "MemoryTrackingInBackgroundProcessingPool",
            "Total amount of memory (bytes) allocated in background processing pool (that is dedicated for backround merges, mutations and fetches). Note that this value may include a drift when the memory was allocated in a context of background processing pool and freed in other context or vice-versa. This happens naturally due to caches for tables indexes and does not indicate memory leaks.",
        ),
        (
            "MemoryTrackingInBackgroundSchedulePool",
            "Total amount of memory (bytes) allocated in background schedule pool (that is dedicated for bookkeeping tasks of Replicated tables).",
        ),
        (
            "MemoryTrackingForMerges",
            "Total amount of memory (bytes) allocated for background merges. Included in MemoryTrackingInBackgroundProcessingPool. Note that this value may include a drift when the memory was allocated in a context of background processing pool and freed in other context or vice-versa. This happens naturally due to caches for tables indexes and does not indicate memory leaks.",
        ),
        (
            "LeaderElection",
            "Number of Replicas participating in leader election. Equals to total number of replicas in usual cases.",
        ),
        ("EphemeralNode", "Number of ephemeral nodes hold in ZooKeeper."),
        (
            "ZooKeeperSession",
            "Number of sessions (connections) to ZooKeeper. Should be no more than one, because using more than one connection to ZooKeeper may lead to bugs due to lack of linearizability (stale reads) that ZooKeeper consistency model allows.",
        ),
        ("ZooKeeperWatch", "Number of watches (event subscriptions) in ZooKeeper."),
        ("ZooKeeperRequest", "Number of requests to ZooKeeper in fly."),
        (
            "DelayedInserts",
            "Number of INSERT queries that are throttled due to high number of active data parts for partition in a MergeTree table.",
        ),
        (
            "ContextLockWait",
            "Number of threads waiting for lock in Context. This is global lock.",
        ),
        ("StorageBufferRows", "Number of rows in buffers of Buffer tables"),
        ("StorageBufferBytes", "Number of bytes in buffers of Buffer tables"),
        (
            "DictCacheRequests",
            "Number of requests in fly to data sources of dictionaries of cache type.",
        ),
        (
            "Revision",
            "Revision of the server. It is a number incremented for every release or release candidate except patch releases.",
        ),
        (
            "VersionInteger",
            "Version of the server in a single integer number in base-1000. For example, version 11.22.33 is translated to 11022033.",
        ),
        (
            "RWLockWaitingReaders",
            "Number of threads waiting for read on a table RWLock.",
        ),
        (
            "RWLockWaitingWriters",
            "Number of threads waiting for write on a table RWLock.",
        ),
        (
            "RWLockActiveReaders",
            "Number of threads holding read lock in a table RWLock.",
        ),
        (
            "RWLockActiveWriters",
            "Number of threads holding write lock in a table RWLock.",
        ),
    ),
    "events": (
        (
            "Query",
            "Number of queries started to be interpreted and maybe executed. Does not include queries that are failed to parse, that are rejected due to AST size limits; rejected due to quota limits or limits on number of simultaneously running queries. May include internal queries initiated by ClickHouse itself. Does not count subqueries.",
        ),
        ("SelectQuery", "Same as Query, but only for SELECT queries."),
        ("FileOpen", "Number of files opened."),
        (
            "ReadBufferFromFileDescriptorRead",
            "Number of reads (read/pread) from a file descriptor. Does not include sockets.",
        ),
        (
            "ReadBufferFromFileDescriptorReadBytes",
            "Number of bytes read from file descriptors. If the file is compressed, this will show compressed data size.",
        ),
        (
            "WriteBufferFromFileDescriptorWrite",
            "Number of writes (write/pwrite) to a file descriptor. Does not include sockets.",
        ),
        (
            "WriteBufferFromFileDescriptorWriteBytes",
            "Number of bytes written to file descriptors. If the file is compressed, this will show compressed data size.",
        ),
        ("ReadCompressedBytes", ""),
        ("CompressedReadBufferBlocks", ""),
        ("CompressedReadBufferBytes", ""),
        ("IOBufferAllocs", ""),
        ("IOBufferAllocBytes", ""),
        ("ArenaAllocChunks", ""),
        ("ArenaAllocBytes", ""),
        ("FunctionExecute", ""),
        (
            "DiskReadElapsedMicroseconds",
            "Total time spent waiting for read syscall. This include reads from page cache.",
        ),
        (
            "DiskWriteElapsedMicroseconds",
            "Total time spent waiting for write syscall. This include writes to page cache.",
        ),
        ("NetworkReceiveElapsedMicroseconds", ""),
        ("NetworkSendElapsedMicroseconds", ""),
        (
            "RegexpCreated",
            "Compiled regular expressions. Identical regular expressions compiled just once and cached forever.",
        ),
        (
            "ContextLock",
            "Number of times the lock of Context was acquired or tried to acquire. This is global lock.",
        ),
        ("RWLockAcquiredReadLocks", ""),
        ("RWLockReadersWaitMilliseconds", ""),
        (
            "RealTimeMicroseconds",
            "Total (wall clock) time spent in processing (queries and other tasks) threads (not that this is a sum).",
        ),
        (
            "UserTimeMicroseconds",
            "Total time spent in processing (queries and other tasks) threads executing CPU instructions in user space. This include time CPU pipeline was stalled due to cache misses, branch mispredictions, hyper-threading, etc.",
        ),
        (
            "SystemTimeMicroseconds",
            "Total time spent in processing (queries and other tasks) threads executing CPU instructions in OS kernel space. This include time CPU pipeline was stalled due to cache misses, branch mispredictions, hyper-threading, etc.",
        ),
        ("SoftPageFaults", ""),
        ("HardPageFaults", ""),
    ),
    "async": (
        ("jemalloc.background_thread.run_interval", ""),
        ("jemalloc.mapped", ""),
        ("jemalloc.resident", ""),
        ("jemalloc.metadata", ""),
        ("jemalloc.active", ""),
        ("jemalloc.background_thread.num_runs", ""),
        ("jemalloc.allocated", ""),
        ("MaxPartCountForPartition", ""),
        ("ReplicasMaxMergesInQueue", ""),
        ("jemalloc.background_thread.num_threads", ""),
        ("ReplicasSumInsertsInQueue", ""),
        ("MarkCacheFiles", ""),
        ("UncompressedCacheBytes", ""),
        ("MarkCacheBytes", ""),
        ("ReplicasSumQueueSize", ""),
        ("jemalloc.metadata_thp", ""),
        ("UncompressedCacheCells", ""),
        ("ReplicasMaxInsertsInQueue", ""),
        ("ReplicasMaxQueueSize", ""),
        ("Uptime", ""),
        ("ReplicasMaxRelativeDelay", ""),
        ("ReplicasSumMergesInQueue", ""),
        ("jemalloc.retained", ""),
        ("ReplicasMaxAbsoluteDelay", ""),
    ),
}


def _fullname(category, name: str) -> str:
    return "{}{}".format(category[0], name).replace(".", "_")


ORDER = []
CHARTS = {}
for category, metrics in _metrics_summary.items():
    metric_type = "incremental" if category == "events" else "absolute"
    chart_type = "line"
    for name, desc in metrics:
        fullname = _fullname(category, name)
        ORDER.append(fullname)
        # Options: None, <description>, <dimension>, <group>, <name>, <chart type>
        fullname2 = "clickhouse." + fullname
        CHARTS[fullname] = {
            "lines": [[fullname, None, metric_type]],
            "options": [None, desc or name, "units", category, fullname2, chart_type],
        }


# Netdata breaks silently on unexpected keys :(
_expected = set()
for c in CHARTS.values():
    for line in c["lines"]:
        _expected.add(line[0])


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        scheme = configuration.get("scheme", "http")
        host = configuration.get("host", "127.0.0.1")
        port = configuration.get("port", 8123)
        self.url = "{0}://{1}:{2}".format(scheme, host, port)

    def _query(self, q: str) -> str:
        """Run a query against the database using HTTP GET"""
        assert self.url
        url = "{0}/?query={1}".format(self.url, q)
        self.debug("starting HTTP request to '{0}'".format(url))
        self.info("starting HTTP request to '{0}'".format(url))
        raw = self._get_raw_data(url)
        if not raw:
            self.error("no data received")
        return raw

    def _query_metrics(self, category: str, q: str) -> dict:
        """Run a query against the database using HTTP GET.
        Expect a tab separated CSV: <metric>\t<integer value>
        Parse the output into a dict
        """
        raw = self._query(q)
        assert raw
        out = {}
        for line in raw.splitlines():
            if not line:
                continue
            name, val = line.split("\t")[0:2]
            fullname = _fullname(category, name)
            out[fullname] = int(val)

        return out

    def _get_data(self) -> dict:
        """Entry point for Netdata
        :returns: dict
        """
        queries = (
            ("events", "SELECT event, value FROM system.events"),
            ("async", "SELECT metric, value FROM system.asynchronous_metrics"),
            ("metrics", "SELECT metric, value FROM system.metrics"),
        )
        # TODO: add table sizes
        # ("table_sizes",
        #     "select concat(database, '.', table), sum(bytes) from system.parts"
        #     " where active = 1 group by database, table",
        # ),
        try:
            # Run all queries across categories
            m = {}
            for category, query in queries:
                new = self._query_metrics(category, query)
                m.update(new)

            discarded = [k for k in m if k not in _expected]
            if discarded:
                # self.info("Discarded: %r" % sorted(discarded))
                self.info("Discarded: %d" % len(discarded))

            out = {k: v for k, v in m.items() if k in _expected}
            return out

        except Exception as e:
            self.error("clickhouse.chart.py error")
            self.error(str(e))
            self.error("end of clickhouse.chart.py error")
            return {}


