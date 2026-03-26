// SPDX-License-Identifier: GPL-3.0-or-later

package relabel

import (
	"strconv"
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"

	"github.com/netdata/netdata/go/plugins/pkg/prometheus/promscrapemodel"
)

var (
	benchSampleSink promscrapemodel.Sample
	benchKeepSink   bool
)

// Current results on ilyam8 workstation (2026-03-25) using:
// `go test ./collector/prometheus/relabel -run ^$ -bench BenchmarkProcessorApply -benchmem`
// goos: darwin
// goarch: arm64
// cpu: Apple M4 Pro
// BenchmarkProcessorApply/no_rules-14              150371938      8.078 ns/op      0 B/op     0 allocs/op
// BenchmarkProcessorApply/keep_passthrough-14       27838136     43.60 ns/op      0 B/op     0 allocs/op
// BenchmarkProcessorApply/name_only_rewrite-14       4569866    254.1 ns/op      56 B/op     3 allocs/op
// BenchmarkProcessorApply/label_replace-14           4475343    281.8 ns/op     208 B/op     4 allocs/op
// BenchmarkProcessorApply/multi_source_replace-14    2411047    469.1 ns/op     320 B/op     7 allocs/op
// BenchmarkProcessorApply/labeldrop_many_labels-14    873496   1346 ns/op       224 B/op     1 allocs/op
//
// These numbers should be updated whenever the relabel hot path changes materially.
func BenchmarkProcessorApply(b *testing.B) {
	type testCase struct {
		name   string
		cfgs   []Config
		sample promscrapemodel.Sample
	}

	tests := []testCase{
		{
			name:   "no_rules",
			cfgs:   nil,
			sample: benchSample("http_requests_total", benchLabels(4), promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			}, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			sample: benchSample("http_requests_total", benchLabels(4), promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			}, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
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
			}, promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
		{
			name: "labeldrop_many_labels",
			cfgs: []Config{
				{
					Regex:  MustNewRegexp("label_[02468]"),
					Action: LabelDrop,
				},
			},
			sample: benchSample("http_requests_total", benchLabels(12), promscrapemodel.SampleKindScalar, commonmodel.MetricTypeCounter),
		},
	}

	for _, test := range tests {
		b.Run(test.name, func(b *testing.B) {
			p, err := New(test.cfgs)
			if err != nil {
				b.Fatalf("New() error = %v", err)
			}

			got, keep := p.Apply(test.sample)
			if !keep {
				b.Fatal("unexpected drop in benchmark setup")
			}
			benchSampleSink = got
			benchKeepSink = keep

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				got, keep = p.Apply(test.sample)
				benchSampleSink = got
				benchKeepSink = keep
			}
		})
	}
}

func benchSample(name string, lbs map[string]string, kind promscrapemodel.SampleKind, familyType commonmodel.MetricType) promscrapemodel.Sample {
	return promscrapemodel.Sample{
		Name:       name,
		Labels:     labels.FromMap(lbs),
		Value:      1,
		Kind:       kind,
		FamilyType: familyType,
	}
}

func benchLabels(n int) map[string]string {
	lbs := make(map[string]string, n)
	for i := 0; i < n; i++ {
		lbs["label_"+strconv.Itoa(i)] = "value"
	}
	return lbs
}
