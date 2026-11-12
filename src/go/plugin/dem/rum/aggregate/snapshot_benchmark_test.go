// SPDX-License-Identifier: GPL-3.0-or-later

package aggregate

import (
	"fmt"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
)

// Sparse activity and attributed measurements exercise the per-read group
// allocation envelope independently of the dense-window benchmark. Capture is
// O(retained observations + facet memberships); sorting is per distribution.
// Timings are local trends, not CI gates. No cross-read state is retained.
func BenchmarkSnapshotPopulations(b *testing.B) {
	for _, shape := range []string{"empty", "activity", "sparse", "attributed"} {
		b.Run(shape, func(b *testing.B) {
			a, now := newAgg(5*time.Minute, SiteCfg{
				Name:       "s",
				PageGroups: 20,
				Countries:  20,
			})
			if shape != "empty" {
				for i := range 10000 {
					input := mk(*now, fmt.Sprint(i), "/page")
					input.PageGroup = fmt.Sprintf("/page_%d", i%1000)
					input.Browser = fmt.Sprintf("browser_%d", i%7)
					input.Country = fmt.Sprintf("country_%d", i%31)
					input.AppVersion = fmt.Sprintf("version_%d", i%37)
					if shape == "attributed" || shape == "sparse" && i%10 == 0 {
						for j, name := range beacon.Vitals {
							if shape == "sparse" && j != 0 {
								break
							}
							value := float64((i*37 + j*23) % 10000)
							if name == beacon.CLS {
								value /= 10000
							}
							metric := v(name, value)
							metric.Element = fmt.Sprintf("#element_%d", i%11)
							input.Vitals = append(input.Vitals, metric)
						}
					}
					a.Ingest(input)
				}
			}
			for _, kind := range []string{"snapshot", "pages"} {
				b.Run(kind, func(b *testing.B) {
					b.ReportAllocs()
					for range b.N {
						if kind == "snapshot" {
							a.Snapshot()
						} else {
							a.Pages("")
						}
					}
				})
			}
		})
	}
}
