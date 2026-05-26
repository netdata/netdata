// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func TestValidateDedupConfig(t *testing.T) {
	tests := map[string]struct {
		cfg     DedupConfig
		wantErr bool
	}{
		"disabled_ignores_zero_values": {
			cfg: DedupConfig{},
		},
		"enabled_uses_defaults": {
			cfg: DedupConfig{Enabled: true},
		},
		"negative_window": {
			cfg: DedupConfig{Enabled: true, WindowSec: -1}, wantErr: true,
		},
		"negative_cache_entries": {
			cfg: DedupConfig{Enabled: true, CacheMaxEntries: -1}, wantErr: true,
		},
		"empty_key_varbind": {
			cfg: DedupConfig{Enabled: true, KeyVarbinds: []string{"ifIndex", " "}}, wantErr: true,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := validateDedupConfig(tc.cfg)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTrapDeduperDefaultFingerprintSuppressesSameSourceAndTrap(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)

	first := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed := d.Admit(first, nil, nil)
	if suppressed {
		t.Fatal("first occurrence was suppressed")
	}

	duplicate := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed = d.Admit(duplicate, nil, nil)
	if !suppressed {
		t.Fatal("duplicate occurrence was not suppressed")
	}

	otherSource := &TrapEntry{SourceIP: "198.51.100.11", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed = d.Admit(otherSource, nil, nil)
	if suppressed {
		t.Fatal("different source was suppressed")
	}
}

func TestTrapDeduperSourceDevicePriority(t *testing.T) {
	t.Run("vnode wins over source IP", func(t *testing.T) {
		d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)

		first := &TrapEntry{SourceVnodeID: "vnode-1", SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
		_, suppressed := d.Admit(first, nil, nil)
		if suppressed {
			t.Fatal("first occurrence was suppressed")
		}

		sameVnodeDifferentIP := &TrapEntry{SourceVnodeID: "vnode-1", SourceIP: "198.51.100.11", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
		_, suppressed = d.Admit(sameVnodeDifferentIP, nil, nil)
		if !suppressed {
			t.Fatal("same vnode with different source IP was not suppressed")
		}

		differentVnodeSameIP := &TrapEntry{SourceVnodeID: "vnode-2", SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
		_, suppressed = d.Admit(differentVnodeSameIP, nil, nil)
		if suppressed {
			t.Fatal("different vnode with same source IP was suppressed")
		}
	})

	t.Run("source IP wins over hostname", func(t *testing.T) {
		d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)

		first := &TrapEntry{SourceIP: "198.51.100.10", DeviceHostname: "core-sw-01", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
		_, suppressed := d.Admit(first, nil, nil)
		if suppressed {
			t.Fatal("first occurrence was suppressed")
		}

		sameIPDifferentHostname := &TrapEntry{SourceIP: "198.51.100.10", DeviceHostname: "core-sw-02", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
		_, suppressed = d.Admit(sameIPDifferentHostname, nil, nil)
		if !suppressed {
			t.Fatal("same source IP with different hostname was not suppressed")
		}
	})
}

func TestTrapDeduperProfileKeyVarbindsNarrowFingerprint(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)
	td := &TrapDef{DedupKeyVarbinds: []string{"ifIndex"}}

	first := dedupTestEntry("198.51.100.10", "1")
	_, suppressed := d.Admit(first, td, nil)
	if suppressed {
		t.Fatal("first occurrence was suppressed")
	}

	differentIfIndex := dedupTestEntry("198.51.100.10", "2")
	_, suppressed = d.Admit(differentIfIndex, td, nil)
	if suppressed {
		t.Fatal("different key varbind value was suppressed")
	}

	sameIfIndex := dedupTestEntry("198.51.100.10", "1")
	_, suppressed = d.Admit(sameIfIndex, td, nil)
	if !suppressed {
		t.Fatal("same key varbind value was not suppressed")
	}
}

func TestTrapDeduperNumericOIDKeyVarbindNarrowFingerprint(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)
	td := &TrapDef{DedupKeyVarbinds: []string{"1.3.6.1.2.1.2.2.1.1.1"}}

	first := dedupTestEntry("198.51.100.10", "1")
	_, suppressed := d.Admit(first, td, nil)
	if suppressed {
		t.Fatal("first occurrence was suppressed")
	}

	differentIfIndex := dedupTestEntry("198.51.100.10", "2")
	_, suppressed = d.Admit(differentIfIndex, td, nil)
	if suppressed {
		t.Fatal("different numeric-OID key varbind value was suppressed")
	}

	sameIfIndex := dedupTestEntry("198.51.100.10", "1")
	_, suppressed = d.Admit(sameIfIndex, td, nil)
	if !suppressed {
		t.Fatal("same numeric-OID key varbind value was not suppressed")
	}
}

func TestTrapDeduperMissingKeyVarbindSentinelDiffersFromEmptyString(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)
	td := &TrapDef{DedupKeyVarbinds: []string{"ifAlias"}}

	empty := &TrapEntry{
		SourceIP: "198.51.100.10",
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		Varbinds: []VarbindValue{{Name: "ifAlias", OID: "1.3.6.1.2.1.31.1.1.1.18.1", Type: "OctetString", Value: ""}},
	}
	_, suppressed := d.Admit(empty, td, nil)
	if suppressed {
		t.Fatal("first empty-string occurrence was suppressed")
	}

	missing := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed = d.Admit(missing, td, nil)
	if suppressed {
		t.Fatal("missing varbind collided with empty-string varbind")
	}

	literalMissing := &TrapEntry{
		SourceIP: "198.51.100.10",
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		Varbinds: []VarbindValue{{Name: "ifAlias", OID: "1.3.6.1.2.1.31.1.1.1.18.1", Type: "OctetString", Value: "<missing>"}},
	}
	_, suppressed = d.Admit(literalMissing, td, nil)
	if suppressed {
		t.Fatal("missing varbind collided with literal <missing> value")
	}
	_, suppressed = d.Admit(literalMissing, td, nil)
	if !suppressed {
		t.Fatal("literal <missing> duplicate was not suppressed")
	}
}

func TestTrapDeduperTTLExpiryAllowsNewFirstOccurrence(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true}, nil, nil)
	d.window = time.Nanosecond

	entry := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	_, suppressed := d.Admit(entry, nil, nil)
	if suppressed {
		t.Fatal("first occurrence was suppressed")
	}
	time.Sleep(time.Millisecond)

	_, suppressed = d.Admit(entry, nil, nil)
	if suppressed {
		t.Fatal("entry remained suppressed after dedup window expiry")
	}
}

func TestTrapDeduperCacheCapEvictsOldestEntry(t *testing.T) {
	d := newTrapDeduper("test", DedupConfig{Enabled: true, CacheMaxEntries: 2}, nil, nil)

	for _, source := range []string{"198.51.100.10", "198.51.100.11", "198.51.100.12"} {
		_, suppressed := d.Admit(&TrapEntry{SourceIP: source, TrapOID: "1.3.6.1.6.3.1.1.5.3"}, nil, nil)
		if suppressed {
			t.Fatalf("first occurrence from %s was suppressed", source)
		}
	}
	if len(d.entries) != 2 {
		t.Fatalf("cache size = %d, want 2", len(d.entries))
	}

	_, suppressed := d.Admit(&TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}, nil, nil)
	if suppressed {
		t.Fatal("oldest entry was not evicted at cap")
	}
}

func TestTrapDeduperSummaryEntry(t *testing.T) {
	writer := &mockTrapWriter{}
	// Test-only shortcut: the deduper summary test does not run Collector.Init(),
	// so it seeds the immutable shared index without touching refcounts.
	globalProfileCache.current.Store(&ProfileIndex{trapsByOID: map[string]*TrapDef{
		"1.3.6.1.6.3.1.1.5.3": {Name: "SNMPv2-MIB::linkDown"},
	}})
	t.Cleanup(func() { globalProfileCache.current.Store(nil) })
	metrics := &perJobMetrics{}
	d := newTrapDeduper("local", DedupConfig{Enabled: true}, writer, metrics)

	entry := &TrapEntry{
		SourceIP:       "198.51.100.10",
		DeviceHostname: "core-sw-01",
		SourceVnodeID:  "source-vnode-id",
		TrapOID:        "1.3.6.1.6.3.1.1.5.3",
	}
	_, suppressed := d.Admit(entry, nil, nil)
	if suppressed {
		t.Fatal("first occurrence was suppressed")
	}
	_, suppressed = d.Admit(entry, nil, nil)
	if !suppressed {
		t.Fatal("second occurrence was not suppressed")
	}
	_, suppressed = d.Admit(entry, nil, nil)
	if !suppressed {
		t.Fatal("third occurrence was not suppressed")
	}

	d.emitSummary(time.Unix(10, 0))

	if len(writer.entries) != 1 {
		t.Fatalf("summary entries = %d, want 1", len(writer.entries))
	}
	summary := writer.entries[0]
	if summary.ReportType != ReportTypeDedupSummary {
		t.Fatalf("ReportType = %q, want deduplication_summary", summary.ReportType)
	}
	if summary.SummaryCounts.TotalSuppressed != 2 {
		t.Fatalf("TotalSuppressed = %d, want 2", summary.SummaryCounts.TotalSuppressed)
	}
	if summary.SummaryCounts.Fingerprints != 1 {
		t.Fatalf("Fingerprints = %d, want 1", summary.SummaryCounts.Fingerprints)
	}
	if !strings.Contains(summary.Message, "SNMPv2-MIB::linkDown 2") {
		t.Fatalf("summary message does not include trap name/count: %q", summary.Message)
	}

	fields, err := serializeToJournalFields(summary)
	if err != nil {
		t.Fatalf("serialize summary: %v", err)
	}
	fieldMap := fieldsToMap(fields)
	if _, ok := fieldMap["_HOSTNAME"]; ok {
		t.Fatal("dedup summary unexpectedly emitted _HOSTNAME")
	}
	if _, ok := fieldMap["ND_NIDL_NODE"]; ok {
		t.Fatal("dedup summary unexpectedly emitted ND_NIDL_NODE")
	}
	if got := atomic.LoadUint64(&metrics.dedup.suppressed); got != 2 {
		t.Fatalf("dedup suppressed metric = %d, want 2", got)
	}
}

func TestTrapDeduperCloseFlushesPendingSummaryAndStopsTimer(t *testing.T) {
	writer := &mockTrapWriter{}
	d := newTrapDeduper("local", DedupConfig{Enabled: true}, writer, nil)
	d.window = time.Hour
	d.start()

	entry := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	d.Admit(entry, nil, nil)
	d.Admit(entry, nil, nil)

	d.Close()

	select {
	case <-d.doneCh:
	default:
		t.Fatal("dedup timer did not stop")
	}
	if len(writer.entries) != 1 {
		t.Fatalf("summary entries after close = %d, want 1", len(writer.entries))
	}
}

func TestTrapDeduperTimerEmitsSummary(t *testing.T) {
	writer := &mockTrapWriter{}
	d := newTrapDeduper("local", DedupConfig{Enabled: true}, writer, nil)
	d.window = 10 * time.Millisecond
	d.start()
	defer d.Close()

	entry := &TrapEntry{SourceIP: "198.51.100.10", TrapOID: "1.3.6.1.6.3.1.1.5.3"}
	d.Admit(entry, nil, nil)
	d.Admit(entry, nil, nil)

	deadline := time.After(500 * time.Millisecond)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		writer.mu.Lock()
		n := len(writer.entries)
		writer.mu.Unlock()
		if n == 1 {
			break
		}

		select {
		case <-deadline:
			t.Fatal("timed out waiting for periodic dedup summary")
		case <-tick.C:
		}
	}

	writer.mu.Lock()
	summary := writer.entries[0]
	writer.mu.Unlock()
	if summary.ReportType != ReportTypeDedupSummary {
		t.Fatalf("ReportType = %q, want deduplication_summary", summary.ReportType)
	}
	if summary.SummaryCounts.TotalSuppressed != 1 {
		t.Fatalf("TotalSuppressed = %d, want 1", summary.SummaryCounts.TotalSuppressed)
	}
}

func TestCollectMetricsDedupSuppressedOnlyWhenEnabled(t *testing.T) {
	const jobName = "test-dedup-metrics"
	removeJobMetrics(jobName)
	defer removeJobMetrics(jobName)

	m := getJobMetrics(jobName)
	m.incDedupSuppressed()

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}

	managed.CycleController().BeginCycle()
	collectMetrics(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit disabled collect cycle: %v", err)
	}
	labels := metrix.Labels{"job_name": jobName}
	if _, ok := store.Read().Value("snmp_trap_dedup_suppressed", labels); ok {
		t.Fatal("dedup metric was emitted while dedup disabled")
	}

	m.setDedupEnabled(true)
	managed.CycleController().BeginCycle()
	collectMetrics(store, jobName)
	if err := managed.CycleController().CommitCycleSuccess(); err != nil {
		t.Fatalf("commit enabled collect cycle: %v", err)
	}
	if v, ok := store.Read().Value("snmp_trap_dedup_suppressed", labels); !ok || v != 1 {
		t.Fatalf("dedup suppressed metric = %v/%v, want 1/true", v, ok)
	}
}

func dedupTestEntry(sourceIP, ifIndex string) *TrapEntry {
	return &TrapEntry{
		SourceIP: sourceIP,
		TrapOID:  "1.3.6.1.6.3.1.1.5.3",
		Varbinds: []VarbindValue{{
			Name:  "ifIndex",
			OID:   "1.3.6.1.2.1.2.2.1.1.1",
			Type:  "Integer",
			Value: ifIndex,
		}},
	}
}
