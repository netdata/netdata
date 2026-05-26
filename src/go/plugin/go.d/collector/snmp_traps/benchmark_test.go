// SPDX-License-Identifier: GPL-3.0-or-later

package snmp_traps

import (
	"runtime"
	"testing"
	"time"
)

func BenchmarkTrapWriterWrite(b *testing.B) {
	tw := newJournalTrapWriter(nil, 1<<20)
	entry := benchmarkTrapEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for {
			err := tw.Write(entry)
			if err == nil {
				break
			}
			if err != errQueueFull {
				b.Fatalf("Write: %v", err)
			}
			runtime.Gosched()
		}
	}
	b.StopTimer()
	_ = tw.Close()
}

func BenchmarkJournalTrapWriterDrain(b *testing.B) {
	requireLinuxJournalBenchmark(b)

	dir := b.TempDir()
	w, err := NewJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	tw := newJournalTrapWriter(w, 1<<20)
	entry := benchmarkTrapEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := tw.Write(entry); err != nil {
			b.Fatalf("Write: %v", err)
		}
	}
	if err := tw.Flush(); err != nil {
		b.Fatalf("Flush: %v", err)
	}
	b.StopTimer()
	if err := tw.Close(); err != nil {
		b.Fatalf("Close: %v", err)
	}
}

func BenchmarkJournalWriterWriteEntry(b *testing.B) {
	requireLinuxJournalBenchmark(b)

	dir := b.TempDir()
	w, err := NewJournalWriter(dir, JournalConfig{RotateSize: 200 * bytesPerMB})
	if err != nil {
		b.Fatalf("NewJournalWriter: %v", err)
	}
	defer w.Close()

	fields, err := serializeToJournalFields(benchmarkTrapEntry())
	if err != nil {
		b.Fatalf("serializeToJournalFields: %v", err)
	}
	now := time.Now().UnixMicro()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := w.WriteEntry(fields, now+int64(i), now+int64(i)); err != nil {
			b.Fatalf("WriteEntry: %v", err)
		}
	}
}

func requireLinuxJournalBenchmark(b *testing.B) {
	b.Helper()
	if runtime.GOOS != "linux" {
		b.Skip("SNMP trap journal backend requires Linux")
	}
}

func benchmarkTrapEntry() *TrapEntry {
	return &TrapEntry{
		JobName:               "bench",
		ReportType:            ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.1",
		TrapName:              "TEST-MIB::coldStart",
		Category:              "security",
		Severity:              "warning",
		Message:               "benchmark trap",
		SourceIP:              "192.0.2.10",
		SourceUDPPeer:         "192.0.2.10",
		PduType:               PduTypeTrap,
		SnmpVersion:           SnmpVersionV2c,
		Labels: map[string]string{
			"site": "lab",
		},
		Varbinds: []VarbindValue{
			{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1", Type: "INTEGER", Value: int64(1)},
			{Name: "ifDescr", OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Ethernet1"},
		},
	}
}
