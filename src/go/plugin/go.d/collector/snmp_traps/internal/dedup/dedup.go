// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

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
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

const (
	varbindMissing = "missing"
	varbindPresent = "present"
)

type key [sha256.Size]byte

type Decision uint8

const (
	DecisionAdmit Decision = iota
	DecisionSuppress
)

type Admission struct {
	key key
	ok  bool
}

type Summary struct {
	ReceivedRealtimeUsec  int64
	ReceivedMonotonicUsec int64
	Message               string
	Counts                *model.DedupSummary
}

type NameResolver func(oid string) string
type SummaryCallback func(Summary)

type Options struct {
	MonotonicNow func() int64
	ResolveName  NameResolver
	OnSummary    SummaryCallback
	Now          func() time.Time
}

type cacheEntry struct {
	key       key
	trapOID   string
	expiresAt time.Time
}

type periodState struct {
	total        int64
	byTrap       map[string]int64
	fingerprints map[key]struct{}
}

type Deduper struct {
	window       time.Duration
	maxEntries   int
	monotonicNow func() int64
	resolveName  NameResolver
	onSummary    SummaryCallback
	now          func() time.Time

	mu      sync.Mutex
	entries map[key]*list.Element
	order   *list.List
	period  periodState

	lifecycleMu sync.Mutex
	started     bool
	closed      bool
	closeCh     chan struct{}
	doneCh      chan struct{}
}

func New(policy Policy, opts Options) *Deduper {
	if !policy.enabled {
		return nil
	}
	if opts.MonotonicNow == nil {
		opts.MonotonicNow = func() int64 { return 0 }
	}
	if opts.ResolveName == nil {
		opts.ResolveName = func(oid string) string { return oid }
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Deduper{
		window:       policy.window,
		maxEntries:   policy.maxEntries,
		monotonicNow: opts.MonotonicNow,
		resolveName:  opts.ResolveName,
		onSummary:    opts.OnSummary,
		now:          opts.Now,
		entries:      make(map[key]*list.Element),
		order:        list.New(),
		period: periodState{
			byTrap:       make(map[string]int64),
			fingerprints: make(map[key]struct{}),
		},
		closeCh: make(chan struct{}),
		doneCh:  make(chan struct{}),
	}
}

func (d *Deduper) Start() {
	if d == nil {
		return
	}
	d.lifecycleMu.Lock()
	defer d.lifecycleMu.Unlock()
	if d.started || d.closed {
		return
	}
	d.started = true
	go d.run()
}

func (d *Deduper) run() {
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
			d.emitSummary(d.now())
			return
		}
	}
}

// Close synchronously completes the final summary callback. The caller may
// close output dependencies as soon as Close returns.
func (d *Deduper) Close() {
	if d == nil {
		return
	}
	d.lifecycleMu.Lock()
	if d.closed {
		d.lifecycleMu.Unlock()
		<-d.doneCh
		return
	}
	d.closed = true
	if !d.started {
		d.lifecycleMu.Unlock()
		defer close(d.doneCh)
		d.emitSummary(d.now())
		return
	}
	close(d.closeCh)
	d.lifecycleMu.Unlock()
	<-d.doneCh
}

func (d *Deduper) Admit(entry *model.TrapEntry, keyVarbinds []string) (Admission, Decision) {
	if d == nil || entry == nil {
		return Admission{}, DecisionAdmit
	}
	now := d.now()
	fingerprint := fingerprint(entry, keyVarbinds)

	d.mu.Lock()
	defer d.mu.Unlock()

	d.evictExpiredLocked(now)
	if elem, ok := d.entries[fingerprint]; ok {
		cacheEntry := elem.Value.(*cacheEntry)
		if now.Before(cacheEntry.expiresAt) {
			d.recordSuppressedLocked(fingerprint, cacheEntry.trapOID)
			return Admission{}, DecisionSuppress
		}
		d.removeElementLocked(elem)
	}

	entryState := &cacheEntry{
		key:       fingerprint,
		trapOID:   entry.TrapOID,
		expiresAt: now.Add(d.window),
	}
	d.entries[fingerprint] = d.order.PushBack(entryState)
	d.trimLocked()
	return Admission{key: fingerprint, ok: true}, DecisionAdmit
}

func (d *Deduper) Rollback(admission Admission) {
	if d == nil || !admission.ok {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if elem, ok := d.entries[admission.key]; ok {
		d.removeElementLocked(elem)
	}
}

func (d *Deduper) recordSuppressedLocked(fingerprint key, trapOID string) {
	if trapOID == "" {
		trapOID = "unknown"
	}
	d.period.total++
	d.period.byTrap[trapOID]++
	d.period.fingerprints[fingerprint] = struct{}{}
}

func (d *Deduper) evictExpiredLocked(now time.Time) {
	for elem := d.order.Front(); elem != nil; {
		next := elem.Next()
		entry := elem.Value.(*cacheEntry)
		if now.Before(entry.expiresAt) {
			return
		}
		d.removeElementLocked(elem)
		elem = next
	}
}

func (d *Deduper) trimLocked() {
	for d.maxEntries > 0 && len(d.entries) > d.maxEntries {
		elem := d.order.Front()
		if elem == nil {
			return
		}
		d.removeElementLocked(elem)
	}
}

func (d *Deduper) removeElementLocked(elem *list.Element) {
	entry := elem.Value.(*cacheEntry)
	delete(d.entries, entry.key)
	d.order.Remove(elem)
}

func (d *Deduper) emitSummary(now time.Time) {
	if d == nil || d.onSummary == nil {
		return
	}
	counts := d.snapshotSummary()
	if counts == nil || counts.TotalSuppressed == 0 {
		return
	}
	d.onSummary(Summary{
		ReceivedRealtimeUsec:  now.UnixMicro(),
		ReceivedMonotonicUsec: d.monotonicNow(),
		Message:               d.renderSummaryMessage(counts),
		Counts:                counts,
	})
}

func (d *Deduper) snapshotSummary() *model.DedupSummary {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.period.total == 0 {
		return nil
	}
	byTrap := make(map[string]int64, len(d.period.byTrap))
	maps.Copy(byTrap, d.period.byTrap)
	summary := &model.DedupSummary{
		TotalSuppressed: d.period.total,
		PeriodSec:       int64(d.window / time.Second),
		Fingerprints:    int64(len(d.period.fingerprints)),
		ByTrap:          byTrap,
	}
	d.period.total = 0
	d.period.byTrap = make(map[string]int64)
	d.period.fingerprints = make(map[key]struct{})
	return summary
}

func (d *Deduper) renderSummaryMessage(summary *model.DedupSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "DEDUPLICATED TRAPS: %d events have been deduplicated in the last %ds:", summary.TotalSuppressed, summary.PeriodSec)
	for _, item := range sortedSummaryItems(summary.ByTrap) {
		fmt.Fprintf(&b, "\n- %s %d", d.resolveName(item.oid), item.count)
	}
	return b.String()
}

type summaryItem struct {
	oid   string
	count int64
}

func sortedSummaryItems(byTrap map[string]int64) []summaryItem {
	items := make([]summaryItem, 0, len(byTrap))
	for oid, count := range byTrap {
		items = append(items, summaryItem{oid: oid, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count != items[j].count {
			return items[i].count > items[j].count
		}
		return items[i].oid < items[j].oid
	})
	return items
}

func fingerprint(entry *model.TrapEntry, keyVarbinds []string) key {
	var stack [512]byte
	buf := stack[:0]
	buf = appendFingerprintPart(buf, "source")
	buf = appendSourceDevice(buf, entry)
	buf = appendFingerprintPart(buf, "trap_oid")
	buf = appendFingerprintPart(buf, entry.TrapOID)

	for _, name := range keyVarbinds {
		buf = appendFingerprintPart(buf, "varbind")
		buf = appendFingerprintPart(buf, name)
		vb, ok := findVarbind(entry, name)
		if !ok {
			buf = appendFingerprintPart(buf, varbindMissing)
			continue
		}
		buf = appendFingerprintPart(buf, varbindPresent)
		buf = appendFingerprintPart(buf, vb.OID)
		buf = appendFingerprintPart(buf, string(vb.Type))
		if model.IsSensitiveVarbind(vb) {
			buf = appendFingerprintValue(buf, model.RedactedVarbindValue)
			continue
		}
		buf = appendFingerprintValue(buf, vb.Value)
	}

	return sha256.Sum256(buf)
}

func appendSourceDevice(buf []byte, entry *model.TrapEntry) []byte {
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

func findVarbind(entry *model.TrapEntry, name string) (model.VarbindValue, bool) {
	if entry == nil {
		return model.VarbindValue{}, false
	}
	if value, ok := model.FindVarbindByName(entry.Varbinds, name); ok {
		return value, true
	}
	if model.IsNumericOID(name) {
		return model.FindVarbindForProfileOID(entry.Varbinds, name)
	}
	return model.VarbindValue{}, false
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
