// SPDX-License-Identifier: GPL-3.0-or-later

package dedup

import (
	"strconv"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

func BenchmarkFingerprint(b *testing.B) {
	entry := benchmarkEntry()
	keys := []string{"ifIndex", "ifDescr"}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_ = fingerprint(entry, keys)
	}
}

func BenchmarkAdmitDuplicate(b *testing.B) {
	entry := benchmarkEntry()
	keys := []string{"ifIndex", "ifDescr"}
	d := newTestDeduper(b, Config{Enabled: true}, frozenClockOptions())
	d.Admit(entry, keys)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		d.Admit(entry, keys)
	}
	b.StopTimer()
	if got := d.period.total; got != int64(b.N) {
		b.Fatalf("suppressed %d entries, want %d", got, b.N)
	}
}

func BenchmarkAdmitUniqueAtCapacity(b *testing.B) {
	const capacity = 4096
	entries := make([]model.TrapEntry, capacity*2)
	for i := range entries {
		entries[i] = model.TrapEntry{
			SourceVnodeID: "vnode-" + strconv.Itoa(i),
			TrapOID:       testTrapOID,
		}
	}
	d := newTestDeduper(b, Config{Enabled: true, CacheMaxEntries: capacity}, frozenClockOptions())
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		d.Admit(&entries[i%len(entries)], nil)
	}
	b.StopTimer()
	if got := d.period.total; got != 0 {
		b.Fatalf("suppressed %d entries in unique-admission workload", got)
	}
}

func BenchmarkAdmitDuplicateParallel(b *testing.B) {
	entry := benchmarkEntry()
	keys := []string{"ifIndex", "ifDescr"}
	d := newTestDeduper(b, Config{Enabled: true}, frozenClockOptions())
	d.Admit(entry, keys)
	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			d.Admit(entry, keys)
		}
	})
	b.StopTimer()
	if got := d.period.total; got != int64(b.N) {
		b.Fatalf("suppressed %d entries, want %d", got, b.N)
	}
}

func BenchmarkRenderSummary(b *testing.B) {
	for _, size := range []int{1, 1000} {
		b.Run(strconv.Itoa(size)+"_trap_oids", func(b *testing.B) {
			byTrap := make(map[string]int64, size)
			for i := range size {
				byTrap["1.3.6.1.4.1.32473."+strconv.Itoa(i)] = int64(i + 1)
			}
			d := newTestDeduper(b, Config{Enabled: true}, Options{})
			summary := &model.DedupSummary{TotalSuppressed: int64(size), PeriodSec: 5, ByTrap: byTrap}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				d.renderSummaryMessage(summary)
			}
		})
	}
}

func benchmarkEntry() *model.TrapEntry {
	return &model.TrapEntry{
		SourceIP: "192.0.2.10",
		TrapOID:  testTrapOID,
		Varbinds: []model.VarbindValue{
			{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1.1", Type: "INTEGER", Value: int64(7)},
			{Name: "ifDescr", OID: "1.3.6.1.2.1.31.1.1.1.1.7", Type: "OctetString", Value: "Gi0/7"},
			{Name: "ifAdminStatus", OID: "1.3.6.1.2.1.2.2.1.7.7", Type: "INTEGER", Value: int64(1)},
			{Name: "ifOperStatus", OID: "1.3.6.1.2.1.2.2.1.8.7", Type: "INTEGER", Value: int64(2)},
		},
	}
}

func frozenClockOptions() Options {
	now := time.Unix(1, 0)
	return Options{Now: func() time.Time { return now }}
}
