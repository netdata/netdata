// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"strconv"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
)

var (
	benchSampleSink prompkg.Sample
	benchKeepSink   bool
)

// relabel runs on every kept sample on the scrape hot path, so keep these
// numbers current before/after changes to the engine. Run with:
//
//	go test ./plugin/go.d/collector/prometheus/relabel -run '^$' -bench BenchmarkProcessorApply -benchmem
//
// Numbers are machine-specific; compare relative before/after deltas, and watch
// the allocation columns (the no-rules and passthrough cases should stay at 0).
func BenchmarkProcessorApply(b *testing.B) {
	type testCase struct {
		name   string
		cfgs   []Config
		sample prompkg.Sample
	}

	tests := []testCase{
		{
			name:   "no_rules",
			cfgs:   nil,
			sample: benchSample("http_requests_total", benchLabels(4), prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "keep_passthrough",
			cfgs: []Config{
				{
					SourceLabels: []string{"job"},
					Regex:        MustNewRegexp("api"),
					Action:       Keep,
				},
			},
			sample: benchSample("http_requests_total", map[string]string{
				"instance": "127.0.0.1:9090",
				"job":      "api",
				"method":   "GET",
				"status":   "200",
			}, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "name_only_rewrite",
			cfgs: []Config{
				{
					SourceLabels: []string{commonmodel.MetricNameLabel},
					Regex:        MustNewRegexp("(.*)_total"),
					TargetLabel:  commonmodel.MetricNameLabel,
					Replacement:  "${1}",
					Action:       Replace,
				},
			},
			sample: benchSample("http_requests_total", benchLabels(4), prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "label_replace",
			cfgs: []Config{
				{
					SourceLabels: []string{"method"},
					TargetLabel:  "http_method",
					Replacement:  "$1",
					Action:       Replace,
				},
			},
			sample: benchSample("http_requests_total", map[string]string{
				"instance": "127.0.0.1:9090",
				"job":      "api",
				"method":   "GET",
				"status":   "200",
			}, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "multi_source_replace",
			cfgs: []Config{
				{
					SourceLabels: []string{"job", "instance", "method", "status"},
					Separator:    "/",
					TargetLabel:  "route_key",
					Replacement:  "$1",
					Action:       Replace,
				},
			},
			sample: benchSample("http_requests_total", map[string]string{
				"instance": "127.0.0.1:9090",
				"job":      "api",
				"method":   "GET",
				"status":   "200",
			}, prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "labeldrop_many_labels",
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("label_[02468]"),
					Action: LabelDrop,
				},
			},
			sample: benchSample("http_requests_total", benchLabels(12), prompkg.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			p, err := New(test.cfgs)
			if err != nil {
				b.Fatalf("New() error = %v", err)
			}

			got, drop := p.Apply(test.sample)
			if drop.Dropped() {
				b.Fatal("unexpected drop in benchmark setup")
			}
			benchSampleSink = got
			benchKeepSink = !drop.Dropped()

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, drop = p.Apply(test.sample)
				benchSampleSink = got
				benchKeepSink = !drop.Dropped()
			}
		})
	}
}

func benchSample(name string, lbs map[string]string, kind prompkg.SampleKind, familyType commonmodel.MetricType) prompkg.Sample {
	return prompkg.Sample{
		Name:       name,
		Labels:     labels.FromMap(lbs),
		Value:      1,
		Kind:       kind,
		FamilyType: familyType,
	}
}

func benchLabels(n int) map[string]string {
	lbs := make(map[string]string, n)
	for i := range n {
		lbs["label_"+strconv.Itoa(i)] = "value"
	}
	return lbs
}
