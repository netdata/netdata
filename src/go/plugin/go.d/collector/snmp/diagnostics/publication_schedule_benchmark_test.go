// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

// Pending polls must remain O(1), allocation-free, independent of device count.
// Timings are workstation trends; the benchmark excludes evidence capture.
func BenchmarkNormalPendingUpdate(b *testing.B) {
	for name, devices := range map[string]int{"one": 1, "thousand": 1000, "ten_thousand": 10000} {
		b.Run(name, func(b *testing.B) {
			p := NewPublisher(ddsnmp.NewDeviceStore(), b.TempDir())
			cut := &normalTestCut{attempt: 1}
			writers := make([]*NormalWriter, devices)
			for i := range writers {
				writers[i] = p.ReplaceNormal("", fmt.Sprint(i), uint64(i+1))
				writers[i].Update(cut)
			}
			b.ReportAllocs()
			b.ResetTimer()
			i := 0
			for b.Loop() {
				writers[i].Update(cut)
				i = (i + 1) % len(writers)
			}
		})
	}
}
