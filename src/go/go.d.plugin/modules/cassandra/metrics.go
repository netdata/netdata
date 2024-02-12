// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#table-metrics
// https://www.datadoghq.com/blog/how-to-collect-cassandra-metrics/
// https://docs.opennms.com/horizon/29/deployment/time-series-storage/newts/cassandra-jmx.html

func newCassandraMetrics() *cassandraMetrics {
	return &cassandraMetrics{
		threadPools: make(map[string]*threadPoolMetrics),
	}
}

type cassandraMetrics struct {
	clientReqTotalLatencyReads  metricValue
	clientReqTotalLatencyWrites metricValue
	clientReqLatencyReads       metricValue
	clientReqLatencyWrites      metricValue
	clientReqTimeoutsReads      metricValue
	clientReqTimeoutsWrites     metricValue
	clientReqUnavailablesReads  metricValue
	clientReqUnavailablesWrites metricValue
	clientReqFailuresReads      metricValue
	clientReqFailuresWrites     metricValue

	clientReqReadLatencyP50   metricValue
	clientReqReadLatencyP75   metricValue
	clientReqReadLatencyP95   metricValue
	clientReqReadLatencyP98   metricValue
	clientReqReadLatencyP99   metricValue
	clientReqReadLatencyP999  metricValue
	clientReqWriteLatencyP50  metricValue
	clientReqWriteLatencyP75  metricValue
	clientReqWriteLatencyP95  metricValue
	clientReqWriteLatencyP98  metricValue
	clientReqWriteLatencyP99  metricValue
	clientReqWriteLatencyP999 metricValue

	rowCacheHits     metricValue
	rowCacheMisses   metricValue
	rowCacheCapacity metricValue
	rowCacheSize     metricValue
	keyCacheHits     metricValue
	keyCacheMisses   metricValue
	keyCacheCapacity metricValue
	keyCacheSize     metricValue

	// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#dropped-metrics
	droppedMessages metricValue

	// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#storage-metrics
	storageLoad       metricValue
	storageExceptions metricValue

	// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#compaction-metrics
	compactionBytesCompacted metricValue
	compactionPendingTasks   metricValue
	compactionCompletedTasks metricValue

	// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#memory
	jvmMemoryHeapUsed    metricValue
	jvmMemoryNonHeapUsed metricValue
	// https://cassandra.apache.org/doc/latest/cassandra/operating/metrics.html#garbagecollector
	jvmGCParNewCount metricValue
	jvmGCParNewTime  metricValue
	jvmGCCMSCount    metricValue
	jvmGCCMSTime     metricValue

	threadPools map[string]*threadPoolMetrics
}

type threadPoolMetrics struct {
	name      string
	hasCharts bool

	activeTasks       metricValue
	pendingTasks      metricValue
	blockedTasks      metricValue
	totalBlockedTasks metricValue
}

type metricValue struct {
	isSet bool
	value float64
}

func (mv *metricValue) add(v float64) {
	mv.isSet = true
	mv.value += v
}

func (mv *metricValue) write(mx map[string]int64, key string) {
	if mv.isSet {
		mx[key] = int64(mv.value)
	}
}

func (mv *metricValue) write1k(mx map[string]int64, key string) {
	if mv.isSet {
		mx[key] = int64(mv.value * 1000)
	}
}
