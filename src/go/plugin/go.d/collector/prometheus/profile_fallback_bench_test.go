// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"fmt"
	"testing"

	commonmodel "github.com/prometheus/common/model"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
)

// BenchmarkProfileFallbackResolution isolates the opt-in per-family matcher
// cost. Jobs with no selected profile fallback bypass this path entirely.
//
// Command:
//
//	go test ./plugin/go.d/collector/prometheus -run '^$' -bench ProfileFallbackResolution -benchmem
//
// A 2026-08-06 Apple M4 Pro run measured 9.5-9.7 ns/op for a first-profile
// hit, 86.3-86.5 ns/op for a sixteenth-profile hit, and 76.1-76.4 ns/op for
// fifteen misses followed by implicit _total. Every case used zero allocations.
func BenchmarkProfileFallbackResolution(b *testing.B) {
	profile := func(root string) profileFallback {
		return profileFallback{
			root:    matcher.Must(matcher.NewSimplePatternsMatcher(root)),
			gauge:   matcher.Must(matcher.NewGlobMatcher("app_value")),
			counter: matcher.FALSE(),
		}
	}
	w := metricFamilyWriter{policy: metricFamilyWriterPolicy{
		isFallbackTypeGauge:   matcher.FALSE(),
		isFallbackTypeCounter: matcher.FALSE(),
	}}

	profilesFirstHit := []profileFallback{profile("app_*")}
	for i := 1; i < 16; i++ {
		profilesFirstHit = append(profilesFirstHit, profile(fmt.Sprintf("other_%d_*", i)))
	}
	profilesLastHit := make([]profileFallback, 0, 16)
	for i := range 15 {
		profilesLastHit = append(profilesLastHit, profile(fmt.Sprintf("other_%d_*", i)))
	}
	profilesLastHit = append(profilesLastHit, profile("app_*"))
	profilesAllMiss := profilesLastHit[:15]

	for _, tc := range []struct {
		name     string
		metric   string
		profiles []profileFallback
		want     commonmodel.MetricType
	}{
		{name: "one_profile_hit", metric: "app_value", profiles: profilesFirstHit[:1], want: commonmodel.MetricTypeGauge},
		{name: "sixteen_profiles_first_hit", metric: "app_value", profiles: profilesFirstHit, want: commonmodel.MetricTypeGauge},
		{name: "sixteen_profiles_last_hit", metric: "app_value", profiles: profilesLastHit, want: commonmodel.MetricTypeGauge},
		{name: "fifteen_profiles_miss_then_implicit_total", metric: "app_value_total", profiles: profilesAllMiss, want: commonmodel.MetricTypeCounter},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for range b.N {
				got, ok := w.resolveType(
					tc.metric,
					commonmodel.MetricTypeUnknown,
					tc.profiles,
					true,
				)
				if !ok || got != tc.want {
					b.Fatalf("resolve type: got (%q, %v), want (%q, true)", got, ok, tc.want)
				}
			}
		})
	}
}
