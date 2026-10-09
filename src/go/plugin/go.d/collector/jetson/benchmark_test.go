// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

// Benchmarks use public records. Parsing scales with record length; writing scales
// with the current readings, never the number of preceding samples. Timings are
// development-machine trends; allocation counts describe the per-record envelope.
func BenchmarkRecord(b *testing.B) {
	for _, name := range []string{"agx-xavier", "orin-r36", "thor-r38.4"} {
		b.Run(name, func(b *testing.B) {
			data, err := os.ReadFile(filepath.Join("testdata", "tegrastats", name+".txt"))
			if err != nil {
				b.Fatal(err)
			}
			line := string(data)
			b.Run("Parse", func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, ok := parseRecord(line); !ok {
						b.Fatal("fixture not recognized")
					}
				}
			})
			b.Run("Write", func(b *testing.B) {
				s, ok := parseRecord(line)
				if !ok {
					b.Fatal("fixture not recognized")
				}
				c := New()
				managed, ok := metrix.AsCycleManagedStore(c.store)
				if !ok {
					b.Fatal("store is not cycle-managed")
				}
				cc := managed.CycleController()
				b.ReportAllocs()
				for b.Loop() {
					cc.BeginCycle()
					c.writeMetrics(s)
					if err := cc.CommitCycleSuccess(); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
