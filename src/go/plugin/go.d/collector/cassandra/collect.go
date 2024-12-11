// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	"errors"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
)

const (
	suffixCount = "_count"
	suffixValue = "_value"
)

func (c *Collector) collect() (map[string]int64, error) {
	pms, err := c.prom.ScrapeSeries()
	if err != nil {
		return nil, err
	}

	if c.validateMetrics {
		if !isCassandraMetrics(pms) {
			return nil, errors.New("collected metrics aren't Collector metrics")
		}
		c.validateMetrics = false
	}

	mx := make(map[string]int64)

	c.resetMetrics()
	c.collectMetrics(pms)
	c.processMetric(mx)

	return mx, nil
}

func (c *Collector) resetMetrics() {
	cm := newCassandraMetrics()
	for key, p := range c.mx.threadPools {
		cm.threadPools[key] = &threadPoolMetrics{
			name:      p.name,
			hasCharts: p.hasCharts,
		}
	}
	c.mx = cm
}

func (c *Collector) processMetric(mx map[string]int64) {
	c.mx.clientReqTotalLatencyReads.write(mx, "client_request_total_latency_reads")
	c.mx.clientReqTotalLatencyWrites.write(mx, "client_request_total_latency_writes")
	c.mx.clientReqLatencyReads.write(mx, "client_request_latency_reads")
	c.mx.clientReqLatencyWrites.write(mx, "client_request_latency_writes")
	c.mx.clientReqTimeoutsReads.write(mx, "client_request_timeouts_reads")
	c.mx.clientReqTimeoutsWrites.write(mx, "client_request_timeouts_writes")
	c.mx.clientReqUnavailablesReads.write(mx, "client_request_unavailables_reads")
	c.mx.clientReqUnavailablesWrites.write(mx, "client_request_unavailables_writes")
	c.mx.clientReqFailuresReads.write(mx, "client_request_failures_reads")
	c.mx.clientReqFailuresWrites.write(mx, "client_request_failures_writes")

	c.mx.clientReqReadLatencyP50.write(mx, "client_request_read_latency_p50")
	c.mx.clientReqReadLatencyP75.write(mx, "client_request_read_latency_p75")
	c.mx.clientReqReadLatencyP95.write(mx, "client_request_read_latency_p95")
	c.mx.clientReqReadLatencyP98.write(mx, "client_request_read_latency_p98")
	c.mx.clientReqReadLatencyP99.write(mx, "client_request_read_latency_p99")
	c.mx.clientReqReadLatencyP999.write(mx, "client_request_read_latency_p999")
	c.mx.clientReqWriteLatencyP50.write(mx, "client_request_write_latency_p50")
	c.mx.clientReqWriteLatencyP75.write(mx, "client_request_write_latency_p75")
	c.mx.clientReqWriteLatencyP95.write(mx, "client_request_write_latency_p95")
	c.mx.clientReqWriteLatencyP98.write(mx, "client_request_write_latency_p98")
	c.mx.clientReqWriteLatencyP99.write(mx, "client_request_write_latency_p99")
	c.mx.clientReqWriteLatencyP999.write(mx, "client_request_write_latency_p999")

	c.mx.rowCacheHits.write(mx, "row_cache_hits")
	c.mx.rowCacheMisses.write(mx, "row_cache_misses")
	c.mx.rowCacheSize.write(mx, "row_cache_size")
	if c.mx.rowCacheHits.isSet && c.mx.rowCacheMisses.isSet {
		if s := c.mx.rowCacheHits.value + c.mx.rowCacheMisses.value; s > 0 {
			mx["row_cache_hit_ratio"] = int64((c.mx.rowCacheHits.value * 100 / s) * 1000)
		} else {
			mx["row_cache_hit_ratio"] = 0
		}
	}
	if c.mx.rowCacheCapacity.isSet && c.mx.rowCacheSize.isSet {
		if s := c.mx.rowCacheCapacity.value; s > 0 {
			mx["row_cache_utilization"] = int64((c.mx.rowCacheSize.value * 100 / s) * 1000)
		} else {
			mx["row_cache_utilization"] = 0
		}
	}

	c.mx.keyCacheHits.write(mx, "key_cache_hits")
	c.mx.keyCacheMisses.write(mx, "key_cache_misses")
	c.mx.keyCacheSize.write(mx, "key_cache_size")
	if c.mx.keyCacheHits.isSet && c.mx.keyCacheMisses.isSet {
		if s := c.mx.keyCacheHits.value + c.mx.keyCacheMisses.value; s > 0 {
			mx["key_cache_hit_ratio"] = int64((c.mx.keyCacheHits.value * 100 / s) * 1000)
		} else {
			mx["key_cache_hit_ratio"] = 0
		}
	}
	if c.mx.keyCacheCapacity.isSet && c.mx.keyCacheSize.isSet {
		if s := c.mx.keyCacheCapacity.value; s > 0 {
			mx["key_cache_utilization"] = int64((c.mx.keyCacheSize.value * 100 / s) * 1000)
		} else {
			mx["key_cache_utilization"] = 0
		}
	}

	c.mx.droppedMessages.write1k(mx, "dropped_messages")

	c.mx.storageLoad.write(mx, "storage_load")
	c.mx.storageExceptions.write(mx, "storage_exceptions")

	c.mx.compactionBytesCompacted.write(mx, "compaction_bytes_compacted")
	c.mx.compactionPendingTasks.write(mx, "compaction_pending_tasks")
	c.mx.compactionCompletedTasks.write(mx, "compaction_completed_tasks")

	c.mx.jvmMemoryHeapUsed.write(mx, "jvm_memory_heap_used")
	c.mx.jvmMemoryNonHeapUsed.write(mx, "jvm_memory_nonheap_used")
	c.mx.jvmGCParNewCount.write(mx, "jvm_gc_parnew_count")
	c.mx.jvmGCParNewTime.write1k(mx, "jvm_gc_parnew_time")
	c.mx.jvmGCCMSCount.write(mx, "jvm_gc_cms_count")
	c.mx.jvmGCCMSTime.write1k(mx, "jvm_gc_cms_time")

	for _, p := range c.mx.threadPools {
		if !p.hasCharts {
			p.hasCharts = true
			c.addThreadPoolCharts(p)
		}

		px := "thread_pool_" + p.name + "_"
		p.activeTasks.write(mx, px+"active_tasks")
		p.pendingTasks.write(mx, px+"pending_tasks")
		p.blockedTasks.write(mx, px+"blocked_tasks")
		p.totalBlockedTasks.write(mx, px+"total_blocked_tasks")
	}
}

func (c *Collector) collectMetrics(pms prometheus.Series) {
	c.collectClientRequestMetrics(pms)
	c.collectDroppedMessagesMetrics(pms)
	c.collectThreadPoolsMetrics(pms)
	c.collectStorageMetrics(pms)
	c.collectCacheMetrics(pms)
	c.collectJVMMetrics(pms)
	c.collectCompactionMetrics(pms)
}

func (c *Collector) collectClientRequestMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_clientrequest"

	var rw struct{ read, write *metricValue }
	for _, pm := range pms.FindByName(metric + suffixCount) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")

		switch name {
		case "TotalLatency":
			rw.read, rw.write = &c.mx.clientReqTotalLatencyReads, &c.mx.clientReqTotalLatencyWrites
		case "Latency":
			rw.read, rw.write = &c.mx.clientReqLatencyReads, &c.mx.clientReqLatencyWrites
		case "Timeouts":
			rw.read, rw.write = &c.mx.clientReqTimeoutsReads, &c.mx.clientReqTimeoutsWrites
		case "Unavailables":
			rw.read, rw.write = &c.mx.clientReqUnavailablesReads, &c.mx.clientReqUnavailablesWrites
		case "Failures":
			rw.read, rw.write = &c.mx.clientReqFailuresReads, &c.mx.clientReqFailuresWrites
		default:
			continue
		}

		switch scope {
		case "Read":
			rw.read.add(pm.Value)
		case "Write":
			rw.write.add(pm.Value)
		}
	}

	rw = struct{ read, write *metricValue }{}

	for _, pm := range pms.FindByNames(
		metric+"_50thpercentile",
		metric+"_75thpercentile",
		metric+"_95thpercentile",
		metric+"_98thpercentile",
		metric+"_99thpercentile",
		metric+"_999thpercentile",
	) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")

		if name != "Latency" {
			continue
		}

		switch {
		case strings.HasSuffix(pm.Name(), "_50thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP50, &c.mx.clientReqWriteLatencyP50
		case strings.HasSuffix(pm.Name(), "_75thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP75, &c.mx.clientReqWriteLatencyP75
		case strings.HasSuffix(pm.Name(), "_95thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP95, &c.mx.clientReqWriteLatencyP95
		case strings.HasSuffix(pm.Name(), "_98thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP98, &c.mx.clientReqWriteLatencyP98
		case strings.HasSuffix(pm.Name(), "_99thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP99, &c.mx.clientReqWriteLatencyP99
		case strings.HasSuffix(pm.Name(), "_999thpercentile"):
			rw.read, rw.write = &c.mx.clientReqReadLatencyP999, &c.mx.clientReqWriteLatencyP999
		default:
			continue
		}

		switch scope {
		case "Read":
			rw.read.add(pm.Value)
		case "Write":
			rw.write.add(pm.Value)
		}
	}
}

func (c *Collector) collectCacheMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_cache"

	var hm struct{ hits, misses *metricValue }
	for _, pm := range pms.FindByName(metric + suffixCount) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")

		switch scope {
		case "KeyCache":
			hm.hits, hm.misses = &c.mx.keyCacheHits, &c.mx.keyCacheMisses
		case "RowCache":
			hm.hits, hm.misses = &c.mx.rowCacheHits, &c.mx.rowCacheMisses
		default:
			continue
		}

		switch name {
		case "Hits":
			hm.hits.add(pm.Value)
		case "Misses":
			hm.misses.add(pm.Value)
		}
	}

	var cs struct{ cap, size *metricValue }
	for _, pm := range pms.FindByName(metric + suffixValue) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")

		switch scope {
		case "KeyCache":
			cs.cap, cs.size = &c.mx.keyCacheCapacity, &c.mx.keyCacheSize
		case "RowCache":
			cs.cap, cs.size = &c.mx.rowCacheCapacity, &c.mx.rowCacheSize
		default:
			continue
		}

		switch name {
		case "Capacity":
			cs.cap.add(pm.Value)
		case "Size":
			cs.size.add(pm.Value)
		}
	}
}

func (c *Collector) collectThreadPoolsMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_threadpools"

	for _, pm := range pms.FindByName(metric + suffixValue) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")
		pool := c.getThreadPoolMetrics(scope)

		switch name {
		case "ActiveTasks":
			pool.activeTasks.add(pm.Value)
		case "PendingTasks":
			pool.pendingTasks.add(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metric + suffixCount) {
		name := pm.Labels.Get("name")
		scope := pm.Labels.Get("scope")
		pool := c.getThreadPoolMetrics(scope)

		switch name {
		case "CompletedTasks":
			pool.totalBlockedTasks.add(pm.Value)
		case "TotalBlockedTasks":
			pool.totalBlockedTasks.add(pm.Value)
		case "CurrentlyBlockedTasks":
			pool.blockedTasks.add(pm.Value)
		}
	}
}

func (c *Collector) collectStorageMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_storage"

	for _, pm := range pms.FindByName(metric + suffixCount) {
		name := pm.Labels.Get("name")

		switch name {
		case "Load":
			c.mx.storageLoad.add(pm.Value)
		case "Exceptions":
			c.mx.storageExceptions.add(pm.Value)
		}
	}
}

func (c *Collector) collectDroppedMessagesMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_droppedmessage"

	for _, pm := range pms.FindByName(metric + suffixCount) {
		c.mx.droppedMessages.add(pm.Value)
	}
}

func (c *Collector) collectJVMMetrics(pms prometheus.Series) {
	const metricMemUsed = "jvm_memory_bytes_used"
	const metricGC = "jvm_gc_collection_seconds"

	for _, pm := range pms.FindByName(metricMemUsed) {
		area := pm.Labels.Get("area")

		switch area {
		case "heap":
			c.mx.jvmMemoryHeapUsed.add(pm.Value)
		case "nonheap":
			c.mx.jvmMemoryNonHeapUsed.add(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricGC + suffixCount) {
		gc := pm.Labels.Get("gc")

		switch gc {
		case "ParNew":
			c.mx.jvmGCParNewCount.add(pm.Value)
		case "ConcurrentMarkSweep":
			c.mx.jvmGCCMSCount.add(pm.Value)
		}
	}

	for _, pm := range pms.FindByName(metricGC + "_sum") {
		gc := pm.Labels.Get("gc")

		switch gc {
		case "ParNew":
			c.mx.jvmGCParNewTime.add(pm.Value)
		case "ConcurrentMarkSweep":
			c.mx.jvmGCCMSTime.add(pm.Value)
		}
	}
}

func (c *Collector) collectCompactionMetrics(pms prometheus.Series) {
	const metric = "org_apache_cassandra_metrics_compaction"

	for _, pm := range pms.FindByName(metric + suffixValue) {
		name := pm.Labels.Get("name")

		switch name {
		case "CompletedTasks":
			c.mx.compactionCompletedTasks.add(pm.Value)
		case "PendingTasks":
			c.mx.compactionPendingTasks.add(pm.Value)
		}
	}
	for _, pm := range pms.FindByName(metric + suffixCount) {
		name := pm.Labels.Get("name")

		switch name {
		case "BytesCompacted":
			c.mx.compactionBytesCompacted.add(pm.Value)
		}
	}
}

func (c *Collector) getThreadPoolMetrics(name string) *threadPoolMetrics {
	pool, ok := c.mx.threadPools[name]
	if !ok {
		pool = &threadPoolMetrics{name: name}
		c.mx.threadPools[name] = pool
	}
	return pool
}

func isCassandraMetrics(pms prometheus.Series) bool {
	for _, pm := range pms {
		if strings.HasPrefix(pm.Name(), "org_apache_cassandra_metrics") {
			return true
		}
	}
	return false
}
