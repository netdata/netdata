// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"maps"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	defaultDedupWindow          = 5 * time.Second
	defaultDedupCacheMaxEntries = 100000
	dedupVarbindMissing         = "missing"
	dedupVarbindPresent         = "present"
)

type dedupKey [sha256.Size]byte

type dedupAdmission struct {
	key dedupKey
	ok  bool
}

type dedupCacheEntry struct {
	key       dedupKey
	trapOID   string
	expiresAt time.Time
}

type dedupPeriodState struct {
	total        int64
	byTrap       map[string]int64
	fingerprints map[dedupKey]struct{}
}

type trapDeduper struct {
	jobName         string
	window          time.Duration
	maxEntries      int
	writer          TrapWriter
	metrics         *perJobMetrics
	writeFailureDim string
	monotonicNow    func() int64

	mu      sync.Mutex
	entries map[dedupKey]*list.Element
	order   *list.List
	period  dedupPeriodState

	closeOnce sync.Once
	startOnce sync.Once
	started   atomic.Bool
	closeCh   chan struct{}
	doneCh    chan struct{}
}

func validateDedupConfig(cfg DedupConfig) error {
	if !cfg.Enabled {
		return nil
	}
	if cfg.WindowSec < 0 {
		return fmt.Errorf("dedup.window_sec must be non-negative, got %d", cfg.WindowSec)
	}
	if cfg.CacheMaxEntries < 0 {
		return fmt.Errorf("dedup.cache_max_entries must be non-negative, got %d", cfg.CacheMaxEntries)
	}
	for i, key := range cfg.KeyVarbinds {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("dedup.key_varbinds[%d] must not be empty", i)
		}
	}
	return nil
}

func newTrapDeduper(jobName string, cfg DedupConfig, writer TrapWriter, metrics *perJobMetrics, writeFailureDim string, monotonicNow ...func() int64) *trapDeduper {
	if !cfg.Enabled {
		return nil
	}
	if writeFailureDim == "" {
		writeFailureDim = trapWriteFailureJournal
	}
	monotonicFn := func() int64 { return 0 }
	if len(monotonicNow) > 0 && monotonicNow[0] != nil {
		monotonicFn = monotonicNow[0]
	}
	window := time.Duration(cfg.WindowSec) * time.Second
	if window <= 0 {
		window = defaultDedupWindow
	}
	maxEntries := cfg.CacheMaxEntries
	if maxEntries <= 0 {
		maxEntries = defaultDedupCacheMaxEntries
	}
	return &trapDeduper{
		jobName:         jobName,
		window:          window,
		maxEntries:      maxEntries,
		writer:          writer,
		metrics:         metrics,
		writeFailureDim: writeFailureDim,
		monotonicNow:    monotonicFn,
		entries:         make(map[dedupKey]*list.Element),
		order:           list.New(),
		period: dedupPeriodState{
			byTrap:       make(map[string]int64),
			fingerprints: make(map[dedupKey]struct{}),
		},
		closeCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (d *trapDeduper) start() {
	if d == nil {
		return
	}
	d.startOnce.Do(func() {
		d.started.Store(true)
		go d.run()
	})
}

func (d *trapDeduper) run() {
	ticker := time.NewTicker(d.window)
	defer func() {
		ticker.Stop()
		close(d.doneCh)
	}()

	for {
		select {
		case now := <-ticker.C:
			d.emitSummary(now)
		case <-d.closeCh:
			d.emitSummary(time.Now())
			return
		}
	}
}

func (d *trapDeduper) Close() {
	if d == nil {
		return
	}
	if !d.started.Load() {
		return
	}
	d.closeOnce.Do(func() {
		close(d.closeCh)
		<-d.doneCh
	})
}

func (d *trapDeduper) Admit(entry *TrapEntry, td *TrapDef, jobKeys []string) (dedupAdmission, bool) {
	if d == nil || entry == nil {
		return dedupAdmission{}, false
	}
	now := time.Now()
	key := dedupFingerprint(entry, td, jobKeys)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.evictExpiredLocked(now)
	if elem, ok := d.entries[key]; ok {
		cacheEntry := elem.Value.(*dedupCacheEntry)
		if now.Before(cacheEntry.expiresAt) {
			d.recordSuppressedLocked(key, cacheEntry.trapOID, entry)
			return dedupAdmission{}, true
		}
		d.removeElementLocked(elem)
	}

	cacheEntry := &dedupCacheEntry{
		key:       key,
		trapOID:   entry.TrapOID,
		expiresAt: now.Add(d.window),
	}
	d.entries[key] = d.order.PushBack(cacheEntry)
	d.trimLocked()
	return dedupAdmission{key: key, ok: true}, false
}

func (d *trapDeduper) Rollback(admission dedupAdmission) {
	if d == nil || !admission.ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if elem, ok := d.entries[admission.key]; ok {
		d.removeElementLocked(elem)
	}
}

func (d *trapDeduper) recordSuppressedLocked(key dedupKey, trapOID string, entry *TrapEntry) {
	if trapOID == "" {
		trapOID = "unknown"
	}
	d.period.total++
	d.period.byTrap[trapOID]++
	d.period.fingerprints[key] = struct{}{}
	if d.metrics != nil {
		d.metrics.incDedupSuppressed()
		d.metrics.recordSourceDedupSuppressed(entry)
	}
}

func (d *trapDeduper) evictExpiredLocked(now time.Time) {
	for elem := d.order.Front(); elem != nil; {
		next := elem.Next()
		cacheEntry := elem.Value.(*dedupCacheEntry)
		if now.Before(cacheEntry.expiresAt) {
			return
		}
		d.removeElementLocked(elem)
		elem = next
	}
}

func (d *trapDeduper) trimLocked() {
	for d.maxEntries > 0 && len(d.entries) > d.maxEntries {
		elem := d.order.Front()
		if elem == nil {
			return
		}
		d.removeElementLocked(elem)
	}
}

func (d *trapDeduper) removeElementLocked(elem *list.Element) {
	cacheEntry := elem.Value.(*dedupCacheEntry)
	delete(d.entries, cacheEntry.key)
	d.order.Remove(elem)
}

func (d *trapDeduper) emitSummary(now time.Time) {
	if d == nil || d.writer == nil {
		return
	}
	summary := d.snapshotSummary()
	if summary == nil || summary.TotalSuppressed == 0 {
		return
	}
	entry := &TrapEntry{
		JobName:               d.jobName,
		ReportType:            ReportTypeDedupSummary,
		ReceivedRealtimeUsec:  now.UnixMicro(),
		ReceivedMonotonicUsec: d.monotonicNow(),
		Message:               d.renderSummaryMessage(summary),
		Severity:              "info",
		SummaryCounts:         summary,
	}
	if err := d.writer.Write(entry); err != nil && d.metrics != nil {
		d.metrics.incError(d.writeFailureDim)
	}
}

func (d *trapDeduper) snapshotSummary() *DedupSummary {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.period.total == 0 {
		return nil
	}
	byTrap := make(map[string]int64, len(d.period.byTrap))
	maps.Copy(byTrap, d.period.byTrap)
	summary := &DedupSummary{
		TotalSuppressed: d.period.total,
		PeriodSec:       int64(d.window / time.Second),
		Fingerprints:    int64(len(d.period.fingerprints)),
		ByTrap:          byTrap,
	}
	d.period.total = 0
	d.period.byTrap = make(map[string]int64)
	d.period.fingerprints = make(map[dedupKey]struct{})
	return summary
}

func (d *trapDeduper) renderSummaryMessage(summary *DedupSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DEDUPLICATED TRAPS: %d events have been deduplicated in the last %ds:", summary.TotalSuppressed, summary.PeriodSec)

	for _, item := range sortedDedupSummaryItems(summary.ByTrap) {
		fmt.Fprintf(&b, "\n- %s %d", d.trapSummaryName(item.oid), item.count)
	}
	return b.String()
}

func (d *trapDeduper) trapSummaryName(oid string) string {
	if idx := CurrentProfileIndex(); idx != nil {
		if td := idx.Lookup(oid); td != nil && td.Name != "" {
			return td.Name
		}
	}
	return oid
}

type dedupSummaryItem struct {
	oid   string
	count int64
}

func sortedDedupSummaryItems(byTrap map[string]int64) []dedupSummaryItem {
	items := make([]dedupSummaryItem, 0, len(byTrap))
	for oid, count := range byTrap {
		items = append(items, dedupSummaryItem{oid: oid, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].oid < items[j].oid
	})
	return items
}

func dedupFingerprint(entry *TrapEntry, td *TrapDef, jobKeys []string) dedupKey {
	var stack [512]byte
	buf := stack[:0]
	buf = appendFingerprintPart(buf, "source")
	buf = appendDedupSourceDevice(buf, entry)
	buf = appendFingerprintPart(buf, "trap_oid")
	buf = appendFingerprintPart(buf, entry.TrapOID)

	for _, name := range dedupKeyVarbinds(td, jobKeys) {
		buf = appendFingerprintPart(buf, "varbind")
		buf = appendFingerprintPart(buf, name)
		vb, ok := dedupVarbind(entry, name)
		if !ok {
			buf = appendFingerprintPart(buf, dedupVarbindMissing)
			continue
		}
		buf = appendFingerprintPart(buf, dedupVarbindPresent)
		buf = appendFingerprintPart(buf, vb.OID)
		buf = appendFingerprintPart(buf, string(vb.Type))
		if isSensitiveTrapVarbind(vb) {
			buf = appendFingerprintValue(buf, redactedTrapVarbind)
			continue
		}
		buf = appendFingerprintValue(buf, vb.Value)
	}

	return sha256.Sum256(buf)
}

func appendDedupSourceDevice(buf []byte, entry *TrapEntry) []byte {
	if entry == nil {
		return appendFingerprintPart(buf, "")
	}
	if entry.SourceVnodeID != "" {
		return appendFingerprintJoined(buf, "vnode:", entry.SourceVnodeID)
	}
	if entry.SourceIP != "" {
		return appendFingerprintJoined(buf, "ip:", entry.SourceIP)
	}
	if entry.SourceUDPPeer != "" {
		return appendFingerprintJoined(buf, "peer:", entry.SourceUDPPeer)
	}
	if entry.DeviceHostname != "" {
		return appendFingerprintJoined(buf, "hostname:", entry.DeviceHostname)
	}
	return appendFingerprintPart(buf, "")
}

func dedupKeyVarbinds(td *TrapDef, jobKeys []string) []string {
	if td != nil && len(td.DedupKeyVarbinds) > 0 {
		return td.DedupKeyVarbinds
	}
	return jobKeys
}

func dedupVarbind(entry *TrapEntry, name string) (VarbindValue, bool) {
	if entry == nil {
		return VarbindValue{}, false
	}
	for _, vb := range entry.Varbinds {
		if vb.Name == name {
			return vb, true
		}
	}
	if isNumericOID(name) {
		return findVarbindForProfileOID(entry, name)
	}
	return VarbindValue{}, false
}

func appendFingerprintValue(buf []byte, val any) []byte {
	switch v := val.(type) {
	case nil:
		buf = appendFingerprintPart(buf, "nil")
	case string:
		buf = appendFingerprintPart(buf, "string")
		buf = appendFingerprintPart(buf, v)
	case int64:
		buf = appendFingerprintPart(buf, "int64")
		buf = appendFingerprintInt(buf, v)
	case uint64:
		buf = appendFingerprintPart(buf, "uint64")
		buf = appendFingerprintUint(buf, v)
	case float64:
		buf = appendFingerprintPart(buf, "float64")
		buf = appendFingerprintFloat(buf, v)
	case bool:
		buf = appendFingerprintPart(buf, "bool")
		buf = appendFingerprintBool(buf, v)
	case []byte:
		buf = appendFingerprintPart(buf, "bytes")
		buf = appendFingerprintHex(buf, v)
	default:
		buf = appendFingerprintPart(buf, "other")
		buf = appendFingerprintPart(buf, fmt.Sprintf("%v", v))
	}
	return buf
}

func appendFingerprintPart(buf []byte, s string) []byte {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	buf = append(buf, lenBuf[:]...)
	return append(buf, s...)
}

func appendFingerprintJoined(buf []byte, prefix, value string) []byte {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(prefix)+len(value)))
	buf = append(buf, lenBuf[:]...)
	buf = append(buf, prefix...)
	return append(buf, value...)
}

func appendFingerprintInt(buf []byte, v int64) []byte {
	var tmp [32]byte
	return appendFingerprintBytes(buf, strconv.AppendInt(tmp[:0], v, 10))
}

func appendFingerprintUint(buf []byte, v uint64) []byte {
	var tmp [32]byte
	return appendFingerprintBytes(buf, strconv.AppendUint(tmp[:0], v, 10))
}

func appendFingerprintFloat(buf []byte, v float64) []byte {
	var tmp [32]byte
	return appendFingerprintBytes(buf, strconv.AppendFloat(tmp[:0], v, 'g', -1, 64))
}

func appendFingerprintBool(buf []byte, v bool) []byte {
	var tmp [5]byte
	return appendFingerprintBytes(buf, strconv.AppendBool(tmp[:0], v))
}

func appendFingerprintHex(buf []byte, v []byte) []byte {
	var lenBuf [8]byte
	hexLen := len(v) * 2
	binary.BigEndian.PutUint64(lenBuf[:], uint64(hexLen))
	buf = append(buf, lenBuf[:]...)
	for _, c := range v {
		buf = append(buf, "0123456789abcdef"[c>>4], "0123456789abcdef"[c&0x0f])
	}
	return buf
}

func appendFingerprintBytes(buf []byte, value []byte) []byte {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(value)))
	buf = append(buf, lenBuf[:]...)
	return append(buf, value...)
}
