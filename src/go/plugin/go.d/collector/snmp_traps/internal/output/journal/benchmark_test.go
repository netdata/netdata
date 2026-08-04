// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
)

func BenchmarkTrapWriterSerializeAndWrite(b *testing.B) {
	writer := newWriter(discardJournalSink{}, Config{}, 1<<20, nil)
	if err := writer.Start(); err != nil {
		b.Fatal(err)
	}
	entry := benchmarkEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeWithBackpressure(b, writer, entry)
	}
	if err := writer.Flush(); err != nil {
		b.Fatalf("flush: %v", err)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
	if err := writer.Close(); err != nil {
		b.Fatalf("close: %v", err)
	}
}

func BenchmarkJournalTrapWriterDrain(b *testing.B) {
	cfg := Config{RotateSize: 200 * bytesPerMB}
	sdk, err := newTestSDKWriter(b.TempDir(), cfg)
	if err != nil {
		b.Fatalf("open SDK journal: %v", err)
	}
	writer := newWriter(sdk, cfg, 1<<20, nil)
	if err := writer.Start(); err != nil {
		b.Fatal(err)
	}
	entry := benchmarkEntry()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		writeWithBackpressure(b, writer, entry)
	}
	if err := writer.Flush(); err != nil {
		b.Fatalf("flush: %v", err)
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
	if err := writer.Close(); err != nil {
		b.Fatalf("close: %v", err)
	}
}

func writeWithBackpressure(b *testing.B, writer *Writer, entry *model.TrapEntry) {
	b.Helper()
	for {
		err := writer.Write(entry)
		if err == nil {
			return
		}
		if err != output.ErrQueueFull {
			b.Fatalf("write: %v", err)
		}
		runtime.Gosched()
	}
}

func benchmarkEntry() *model.TrapEntry {
	return &model.TrapEntry{
		JobName:               "bench",
		ReportType:            model.ReportTypeTrap,
		ReceivedRealtimeUsec:  1000000,
		ReceivedMonotonicUsec: 1000,
		TrapOID:               "1.3.6.1.6.3.1.1.5.1",
		TrapName:              "TEST-MIB::coldStart",
		Category:              "security",
		Severity:              "warning",
		Message:               "benchmark trap",
		SourceIP:              "192.0.2.10",
		SourceUDPPeer:         "192.0.2.10",
		PduType:               model.PduTypeTrap,
		SnmpVersion:           model.SnmpVersionV2c,
		Labels:                map[string]string{"site": "lab"},
		Varbinds: []model.VarbindValue{
			{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1", Type: "INTEGER", Value: int64(1)},
			{Name: "ifDescr", OID: "1.3.6.1.2.1.31.1.1.1.1", Type: "OctetString", Value: "Ethernet1"},
		},
	}
}
