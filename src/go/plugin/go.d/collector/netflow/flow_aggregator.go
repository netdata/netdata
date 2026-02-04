// SPDX-License-Identifier: GPL-3.0-or-later

package netflow

import (
	"sort"
	"sync"
	"time"
)

type flowAggregatorConfig struct {
	bucketDuration time.Duration
	maxBuckets     int
	maxKeys        int
	defaultRate    int
	exporters      []ExporterConfig
}

type flowAggregator struct {
	mu sync.RWMutex

	bucketDuration time.Duration
	maxBuckets     int
	maxKeys        int
	defaultRate    int

	buckets map[time.Time]*flowBucketState

	exporterOverrides map[string]exporterOverride
	exporters         map[string]*exporterState

	stats flowCollectorStats

	now func() time.Time
}

type flowBucketState struct {
	start   time.Time
	entries map[flowBucketKey]*flowBucketEntry
	stats   bucketStats
}

type bucketStats struct {
	droppedRecords uint64
}

type flowBucketKey struct {
	Key        flowKey
	ExporterIP string
	Direction  string
}

type flowBucketEntry struct {
	Key          flowKey
	ExporterIP   string
	Direction    string
	Bytes        uint64
	Packets      uint64
	Flows        uint64
	RawBytes     uint64
	RawPackets   uint64
	SamplingRate int
}

type exporterOverride struct {
	name         string
	samplingRate int
}

type exporterState struct {
	ip          string
	name        string
	samplingRate int
	flowVersion string
	lastSeen    time.Time
}

type flowCollectorStats struct {
	recordsTotal   uint64
	recordsDropped uint64
	recordsTooOld  uint64
	decodeErrors   uint64
}

type flowTotals struct {
	Bytes   uint64
	Packets uint64
	Flows   uint64
	Dropped uint64
	Duration time.Duration
	Timestamp time.Time
}

func newFlowAggregator(cfg flowAggregatorConfig) *flowAggregator {
	a := &flowAggregator{
		bucketDuration: cfg.bucketDuration,
		maxBuckets:     cfg.maxBuckets,
		maxKeys:        cfg.maxKeys,
		defaultRate:    cfg.defaultRate,
		buckets:        make(map[time.Time]*flowBucketState),
		exporterOverrides: make(map[string]exporterOverride),
		exporters:      make(map[string]*exporterState),
		now:            time.Now,
	}

	if a.bucketDuration <= 0 {
		a.bucketDuration = 10 * time.Second
	}
	if a.maxBuckets <= 0 {
		a.maxBuckets = defaultMaxBuckets
	}
	if a.maxKeys < 0 {
		a.maxKeys = 0
	}
	if a.defaultRate <= 0 {
		a.defaultRate = 1
	}

	for _, exp := range cfg.exporters {
		if exp.IP == "" {
			continue
		}
		override := exporterOverride{name: exp.Name, samplingRate: exp.SamplingRate}
		a.exporterOverrides[exp.IP] = override
	}

	return a
}

func (a *flowAggregator) RecordDecodeError() {
	a.mu.Lock()
	a.stats.decodeErrors++
	a.mu.Unlock()
}

func (a *flowAggregator) AddRecords(records []flowRecord) {
	if len(records) == 0 {
		return
	}
	for i := range records {
		a.addRecord(&records[i])
	}
}

func (a *flowAggregator) addRecord(record *flowRecord) {
	if record == nil {
		return
	}

	now := a.now()
	if record.Timestamp.IsZero() {
		record.Timestamp = now
	}
	bucketStart := record.Timestamp.Truncate(a.bucketDuration)
	if a.maxBuckets > 0 {
		cutoff := now.Add(-time.Duration(a.maxBuckets) * a.bucketDuration)
		if bucketStart.Before(cutoff) {
			a.mu.Lock()
			a.stats.recordsTooOld++
			a.mu.Unlock()
			record.Timestamp = now
			bucketStart = record.Timestamp.Truncate(a.bucketDuration)
		}
	}

	exporterIP := record.ExporterIP
	if exporterIP == "" {
		exporterIP = "unknown"
	}

	effectiveRate := record.SamplingRate
	if effectiveRate <= 0 {
		if override, ok := a.exporterOverrides[exporterIP]; ok && override.samplingRate > 0 {
			effectiveRate = override.samplingRate
		} else {
			effectiveRate = a.defaultRate
		}
	}
	if effectiveRate <= 0 {
		effectiveRate = 1
	}

	record.RawBytes = record.Bytes
	record.RawPackets = record.Packets
	record.SamplingRate = effectiveRate

	record.Bytes = record.Bytes * uint64(effectiveRate)
	record.Packets = record.Packets * uint64(effectiveRate)
	if record.Flows == 0 {
		record.Flows = 1
	}
	record.Flows = record.Flows * uint64(effectiveRate)

	bucketKey := flowBucketKey{Key: record.Key, ExporterIP: exporterIP, Direction: record.Direction}

	a.mu.Lock()
	defer a.mu.Unlock()

	bucket := a.getBucketLocked(bucketStart)
	if bucket == nil {
		return
	}

	entry := bucket.entries[bucketKey]
	if entry == nil {
		if a.maxKeys > 0 && len(bucket.entries) >= a.maxKeys {
			bucket.stats.droppedRecords++
			a.stats.recordsDropped++
			return
		}
		entry = &flowBucketEntry{Key: record.Key, ExporterIP: exporterIP, Direction: record.Direction}
		bucket.entries[bucketKey] = entry
	}

	entry.Bytes += record.Bytes
	entry.Packets += record.Packets
	entry.Flows += record.Flows
	entry.RawBytes += record.RawBytes
	entry.RawPackets += record.RawPackets
	entry.SamplingRate = record.SamplingRate

	if record.FlowVersion != "" {
		a.updateExporterLocked(exporterIP, record.FlowVersion, record.SamplingRate)
	}

	a.stats.recordsTotal++
}

func (a *flowAggregator) updateExporterLocked(ip, version string, samplingRate int) {
	state := a.exporters[ip]
	if state == nil {
		state = &exporterState{ip: ip}
		if override, ok := a.exporterOverrides[ip]; ok {
			state.name = override.name
			if samplingRate <= 0 {
				samplingRate = override.samplingRate
			}
		}
		a.exporters[ip] = state
	}
	state.flowVersion = version
	if samplingRate > 0 {
		state.samplingRate = samplingRate
	}
	state.lastSeen = a.now()
}

func (a *flowAggregator) getBucketLocked(start time.Time) *flowBucketState {
	bucket := a.buckets[start]
	if bucket != nil {
		return bucket
	}

	if a.maxBuckets > 0 && len(a.buckets) >= a.maxBuckets {
		oldest := start
		for ts := range a.buckets {
			if ts.Before(oldest) {
				oldest = ts
			}
		}
		delete(a.buckets, oldest)
	}

	bucket = &flowBucketState{start: start, entries: make(map[flowBucketKey]*flowBucketEntry)}
	a.buckets[start] = bucket
	return bucket
}

func (a *flowAggregator) Snapshot(agentID string) flowData {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var starts []time.Time
	for start := range a.buckets {
		starts = append(starts, start)
	}
	sort.Slice(starts, func(i, j int) bool { return starts[i].Before(starts[j]) })

	buckets := make([]flowBucket, 0, len(starts))
	summaries := map[string]any{
		"total_bytes":   uint64(0),
		"total_packets": uint64(0),
		"total_flows":   uint64(0),
		"raw_bytes":     uint64(0),
		"raw_packets":   uint64(0),
		"dropped_records": a.stats.recordsDropped,
	}

	for _, start := range starts {
		bucket := a.buckets[start]
		if bucket == nil {
			continue
		}
		for _, entry := range bucket.entries {
			bucketItem := flowBucket{
				Timestamp:    bucket.start,
				DurationSec:  int(a.bucketDuration.Seconds()),
				Key:          &entry.Key,
				Bytes:        entry.Bytes,
				Packets:      entry.Packets,
				Flows:        entry.Flows,
				RawBytes:     entry.RawBytes,
				RawPackets:   entry.RawPackets,
				SamplingRate: entry.SamplingRate,
				Direction:    entry.Direction,
				ExporterIP:   entry.ExporterIP,
				AgentID:      agentID,
			}
			buckets = append(buckets, bucketItem)

			summaries["total_bytes"] = summaries["total_bytes"].(uint64) + entry.Bytes
			summaries["total_packets"] = summaries["total_packets"].(uint64) + entry.Packets
			summaries["total_flows"] = summaries["total_flows"].(uint64) + entry.Flows
			summaries["raw_bytes"] = summaries["raw_bytes"].(uint64) + entry.RawBytes
			summaries["raw_packets"] = summaries["raw_packets"].(uint64) + entry.RawPackets
		}
	}

	exporters := make([]flowExporter, 0, len(a.exporters))
	for _, state := range a.exporters {
		exporters = append(exporters, flowExporter{
			ExporterIP:   state.ip,
			ExporterName: state.name,
			SamplingRate: state.samplingRate,
			FlowVersion:  state.flowVersion,
		})
	}
	sort.Slice(exporters, func(i, j int) bool { return exporters[i].ExporterIP < exporters[j].ExporterIP })

	var periodStart, periodEnd time.Time
	if len(starts) > 0 {
		periodStart = starts[0]
		periodEnd = starts[len(starts)-1].Add(a.bucketDuration)
	} else {
		periodEnd = a.now()
		periodStart = periodEnd.Add(-a.bucketDuration)
	}

	metrics := map[string]any{
		"records_total":   a.stats.recordsTotal,
		"records_dropped": a.stats.recordsDropped,
		"records_too_old": a.stats.recordsTooOld,
		"decode_errors":   a.stats.decodeErrors,
		"exporters":        len(a.exporters),
	}

	return flowData{
		SchemaVersion: flowSchemaVersion,
		AgentID:       agentID,
		PeriodStart:   periodStart,
		PeriodEnd:     periodEnd,
		Exporters:     exporters,
		Buckets:       buckets,
		Summaries:     summaries,
		Metrics:       metrics,
	}
}

func (a *flowAggregator) LatestTotals() flowTotals {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var latest *flowBucketState
	for _, bucket := range a.buckets {
		if latest == nil || bucket.start.After(latest.start) {
			latest = bucket
		}
	}

	if latest == nil {
		return flowTotals{}
	}

	var totals flowTotals
	for _, entry := range latest.entries {
		totals.Bytes += entry.Bytes
		totals.Packets += entry.Packets
		totals.Flows += entry.Flows
	}
	totals.Dropped = latest.stats.droppedRecords
	totals.Duration = a.bucketDuration
	totals.Timestamp = latest.start
	return totals
}
