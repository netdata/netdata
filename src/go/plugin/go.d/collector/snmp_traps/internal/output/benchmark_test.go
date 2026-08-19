// SPDX-License-Identifier: GPL-3.0-or-later

package output

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_traps/internal/model"
)

type discardWriter struct{}

func (discardWriter) Write(*model.TrapEntry) error { return nil }
func (discardWriter) Flush() error                 { return nil }
func (discardWriter) Close() error                 { return nil }

func BenchmarkCoordinatorWrite(b *testing.B) {
	writer := NewCoordinator(discardWriter{}, discardWriter{}, BackendOTLP, nil)
	entry := &model.TrapEntry{}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := writer.Write(entry); err != nil {
			b.Fatal(err)
		}
	}
}
