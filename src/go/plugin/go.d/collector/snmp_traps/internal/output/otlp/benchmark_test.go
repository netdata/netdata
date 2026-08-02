// SPDX-License-Identifier: GPL-3.0-or-later

package otlp

import (
	"runtime"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/output"
)

func BenchmarkWriterWrite(b *testing.B) {
	writer := &Writer{
		queue:   make(chan *model.TrapEntry, 1024),
		started: true,
	}
	done := make(chan struct{})
	go func() {
		for range writer.queue {
		}
		close(done)
	}()
	entry := testTrapEntry()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for {
			err := writer.Write(entry)
			if err == nil {
				break
			}
			if err != output.ErrQueueFull {
				b.Fatalf("write: %v", err)
			}
			runtime.Gosched()
		}
	}
	b.StopTimer()
	close(writer.queue)
	<-done
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "entries/s")
}

func BenchmarkTrapEntryToOTLPLogRecord(b *testing.B) {
	entry := testTrapEntry()
	entry.Labels = map[string]string{"site": "lab", "interface": "eth0"}
	entry.Varbinds = []model.VarbindValue{
		{Name: "ifIndex", OID: "1.3.6.1.2.1.2.2.1.1.1", Type: "INTEGER", Value: int64(1)},
		{Name: "ifDescr", OID: "1.3.6.1.2.1.2.2.1.2.1", Type: "OctetString", Value: "eth0"},
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := trapEntryToOTLPLogRecord(entry); err != nil {
			b.Fatal(err)
		}
	}
}
