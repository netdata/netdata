// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/require"
)

// BenchmarkCollectInputAdmission includes fetch, parse, admission, writer and store
// commit. File input removes network scheduling noise. Admission is O(assembled
// metric entries), with constant-time bucket/quantile counts and no allocations.
func BenchmarkCollectInputAdmission(b *testing.B) {
	for _, population := range []int{50, 500} {
		for _, limit := range []int{0, 10000} {
			b.Run(fmt.Sprintf("series_per_type_%d/limit_%d", population, limit), func(b *testing.B) {
				path := filepath.Join(b.TempDir(), "metrics.prom")
				require.NoError(b, os.WriteFile(path, []byte(buildBenchExposition(population)), 0600))
				c := New()
				c.URL = "file://" + path
				c.Profiles.Mode = profilesModeNone
				c.MaxInputSeries = limit
				ctx := context.Background()
				require.NoError(b, c.Init(ctx))
				defer c.Cleanup(ctx)
				require.NoError(b, c.Check(ctx))
				managed, ok := metrix.AsCycleManagedStore(c.store)
				require.True(b, ok)
				cc := managed.CycleController()
				b.ReportAllocs()
				b.ResetTimer()
				for b.Loop() {
					cc.BeginCycle()
					if err := c.Collect(ctx); err != nil {
						b.Fatal(err)
					}
					if err := cc.CommitCycleSuccess(); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkInputAdmission(b *testing.B) {
	path := filepath.Join(b.TempDir(), "metrics.prom")
	require.NoError(b, os.WriteFile(path, []byte(buildBenchExposition(500)), 0600))
	c := New()
	c.URL = "file://" + path
	c.Profiles.Mode = profilesModeNone
	require.NoError(b, c.Init(context.Background()))
	defer c.Cleanup(context.Background())
	families, err := c.prom.Scrape()
	require.NoError(b, err)
	for _, limit := range []int{0, 10000, 1} {
		b.Run(fmt.Sprintf("limit_%d", limit), func(b *testing.B) {
			c.MaxInputSeries = limit
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = c.admitInput(families)
			}
		})
	}
}
