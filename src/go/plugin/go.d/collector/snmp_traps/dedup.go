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

func newTrapDeduper(jobName string, cfg DedupConfig, writer TrapWriter, metrics *perJobMetrics, writeFailureDim string) *trapDeduper {
	if !cfg.Enabled {
		return nil
	}
	if writeFailureDim == "" {
		writeFailureDim = trapWriteFailureJournal
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
			d.recordSuppressedLocked(key, cacheEntry.trapOID)
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

func (d *trapDeduper) recordSuppressedLocked(key dedupKey, trapOID string) {
	if trapOID == "" {
		trapOID = "unknown"
	}
	d.period.total++
	d.period.byTrap[trapOID]++
	d.period.fingerprints[key] = struct{}{}
	if d.metrics != nil {
		d.metrics.incDedupSuppressed()
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
		ReceivedMonotonicUsec: monotonicUsec(),
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
	h := sha256.New()
	writeFingerprintPart(h, "source")
	writeFingerprintPart(h, dedupSourceDevice(entry))
	writeFingerprintPart(h, "trap_oid")
	writeFingerprintPart(h, entry.TrapOID)

	for _, name := range dedupKeyVarbinds(td, jobKeys) {
		writeFingerprintPart(h, "varbind")
		writeFingerprintPart(h, name)
		vb, ok := dedupVarbind(entry, name)
		if !ok {
			writeFingerprintPart(h, dedupVarbindMissing)
			continue
		}
		writeFingerprintPart(h, dedupVarbindPresent)
		writeFingerprintPart(h, vb.OID)
		writeFingerprintPart(h, string(vb.Type))
		writeFingerprintValue(h, canonicalVarbindValue(vb.Value))
	}

	var out dedupKey
	copy(out[:], h.Sum(nil))
	return out
}

func dedupSourceDevice(entry *TrapEntry) string {
	if entry == nil {
		return ""
	}
	if entry.SourceVnodeID != "" {
		return "vnode:" + entry.SourceVnodeID
	}
	if entry.SourceIP != "" {
		return "ip:" + entry.SourceIP
	}
	if entry.SourceUDPPeer != "" {
		return "peer:" + entry.SourceUDPPeer
	}
	if entry.DeviceHostname != "" {
		return "hostname:" + entry.DeviceHostname
	}
	return ""
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
		if vb.Name == name || (isNumericOID(name) && vb.OID == name) {
			return vb, true
		}
	}
	return VarbindValue{}, false
}

func writeFingerprintValue(h fingerprintWriter, val any) {
	switch v := val.(type) {
	case nil:
		writeFingerprintPart(h, "nil")
	case string:
		writeFingerprintPart(h, "string")
		writeFingerprintPart(h, v)
	case int64:
		writeFingerprintPart(h, "int64")
		writeFingerprintPart(h, strconv.FormatInt(v, 10))
	case uint64:
		writeFingerprintPart(h, "uint64")
		writeFingerprintPart(h, strconv.FormatUint(v, 10))
	case float64:
		writeFingerprintPart(h, "float64")
		writeFingerprintPart(h, strconv.FormatFloat(v, 'g', -1, 64))
	case bool:
		writeFingerprintPart(h, "bool")
		writeFingerprintPart(h, strconv.FormatBool(v))
	default:
		writeFingerprintPart(h, "other")
		writeFingerprintPart(h, fmt.Sprintf("%v", v))
	}
}

type fingerprintWriter interface {
	Write([]byte) (int, error)
}

func writeFingerprintPart(h fingerprintWriter, s string) {
	var lenBuf [8]byte
	binary.BigEndian.PutUint64(lenBuf[:], uint64(len(s)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(s))
}
