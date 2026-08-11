// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	promselector "github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

func TestPipelineDiagnosticsDoNotChangeNoRelabelOutput(t *testing.T) {
	dumpPath := filepath.Join(t.TempDir(), "metrics.txt")
	require.NoError(t, os.WriteFile(dumpPath, []byte(`
# TYPE app_gauge gauge
app_gauge{id="a"} 1
# TYPE app_counter_total counter
app_counter_total{id="a"} 2
# TYPE app_summary summary
app_summary{quantile="0.5"} 3
app_summary_sum 4
app_summary_count 5
# TYPE app_histogram histogram
app_histogram_bucket{le="1"} 6
app_histogram_bucket{le="+Inf"} 7
app_histogram_sum 8
app_histogram_count 7
`), 0o600))

	withoutDiagnostics := collectPipelineSnapshot(t, "file://"+dumpPath)
	withDiagnostics := collectPipelineSnapshot(t, "file://"+dumpPath, WithPipelineDiagnosticObserver(func(PipelineDiagnostic) {}))
	assert.Equal(t, withoutDiagnostics, withDiagnostics)
}

func TestPipelineDiagnosticsDoNotChangeRelabelOutput(t *testing.T) {
	dumpPath := filepath.Join(t.TempDir(), "metrics.txt")
	require.NoError(t, os.WriteFile(dumpPath, []byte(`
# TYPE raw_keep gauge
raw_keep 1
# TYPE raw_drop gauge
raw_drop 2
`), 0o600))
	configure := func(collector *Collector) {
		collector.Relabeling = []relabel.Block{
			{
				Match: "raw_*",
				MetricRelabelConfigs: []relabel.Config{{
					SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp(`raw_(.*)`),
					TargetLabel: "__name__", Replacement: `app_${1}`, Action: relabel.Replace,
				}},
			},
			{
				Match: "app_*",
				MetricRelabelConfigs: []relabel.Config{{
					TargetLabel: "stage", Replacement: "validated", Action: relabel.Replace,
				}},
			},
			{
				Match: "app_drop",
				MetricRelabelConfigs: []relabel.Config{{
					SourceLabels: []string{"__name__"}, Regex: relabel.MustNewRegexp(`app_drop`), Action: relabel.Drop,
				}},
			},
		}
	}

	withoutDiagnostics := collectConfiguredPipelineSnapshot(t, "file://"+dumpPath, configure)
	withDiagnostics := collectConfiguredPipelineSnapshot(
		t, "file://"+dumpPath, configure, WithPipelineDiagnosticObserver(func(PipelineDiagnostic) {}),
	)
	assert.Equal(t, withoutDiagnostics, withDiagnostics)
}

func TestWriterPipelineDiagnosticDistinguishesInvalidValue(t *testing.T) {
	var facts []PipelineDiagnostic
	store := metrix.NewCollectorStore()
	writer := newMetricFamilyWriter(
		store,
		metricFamilyWriterPolicy{observePipeline: func(fact PipelineDiagnostic) { facts = append(facts, fact) }},
		logger.New(),
	)
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	managed.CycleController().BeginCycle()

	written := writer.writeMetricFamilies(scrape(t, `
# TYPE app_value gauge
app_value{id="valid"} 1
app_value{id="invalid"} NaN
`))
	require.Equal(t, 1, written)
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineWriterSeriesRejected &&
			fact.Reason == PipelineReasonInvalidSeriesValue &&
			fact.Destination.Family == "app_value"
	})
}

func TestWriterPipelineDiagnosticClassifiesAllInvalidValues(t *testing.T) {
	tests := map[string]string{
		"gauge": `
# TYPE app_value gauge
app_value NaN
`,
		"histogram": `
# TYPE app_value histogram
app_value_bucket{le="1"} 1
app_value_bucket{le="+Inf"} 1
app_value_sum +Inf
app_value_count 1
`,
		"summary": `
# TYPE app_value summary
app_value{quantile="0.5"} NaN
app_value_sum 0
app_value_count 0
`,
	}

	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			var facts []PipelineDiagnostic
			store := metrix.NewCollectorStore()
			writer := newMetricFamilyWriter(
				store,
				metricFamilyWriterPolicy{observePipeline: func(fact PipelineDiagnostic) { facts = append(facts, fact) }},
				logger.New(),
			)
			managed, ok := metrix.AsCycleManagedStore(store)
			require.True(t, ok)
			managed.CycleController().BeginCycle()

			assert.Zero(t, writer.writeMetricFamilies(scrape(t, input)))
			assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
				return fact.Decision == PipelineWriterFamilyRejected &&
					fact.Reason == PipelineReasonInvalidSeriesValue &&
					fact.MetricName == "app_value"
			})
		})
	}
}

func TestPipelineDiagnosticsFollowRealCollectorDecisions(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"candidate": `
match: app_*
template:
  family: Test
  context_namespace: app
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: value
      dimensions:
        - selector: app_value
          name: value
`,
	})
	dumpPath := filepath.Join(t.TempDir(), "metrics.txt")
	require.NoError(t, os.WriteFile(dumpPath, []byte(`
# TYPE raw_value gauge
raw_value{state="keep"} 1
raw_value{state="drop"} 2
# TYPE raw_many gauge
raw_many{id="a"} 1
raw_many{id="b"} 2
# TYPE ignored gauge
ignored 1
`), 0o600))

	var facts []PipelineDiagnostic
	collector := NewWithOptions(
		WithProfileCatalog(catalog),
		WithPipelineDiagnosticObserver(func(fact PipelineDiagnostic) {
			facts = append(facts, fact)
		}),
	)
	collector.URL = "file://" + dumpPath
	collector.Selector = promselector.Expr{Allow: []string{"raw_*"}}
	collector.MaxTSPerMetric = 1
	collector.Relabeling = []relabel.Block{{
		Match: "raw_*",
		MetricRelabelConfigs: []relabel.Config{
			{
				SourceLabels: []string{"__name__"},
				Regex:        relabel.MustNewRegexp("raw_(.*)"),
				TargetLabel:  "__name__",
				Replacement:  "app_${1}",
				Action:       relabel.Replace,
			},
			{
				SourceLabels: []string{"state"},
				Regex:        relabel.MustNewRegexp("drop"),
				Action:       relabel.Drop,
			},
		},
	}}
	collector.Profiles = ProfilesConfig{
		Mode: profilesModeExact,
		ModeExact: &ProfilesModeConfig{
			Entries: []ProfileEntryConfig{{Name: "candidate"}},
		},
	}

	ctx := context.Background()
	require.NoError(t, collector.Init(ctx))
	defer collector.Cleanup(ctx)
	require.NoError(t, collector.Check(ctx))

	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(ctx))
	require.NoError(t, cycle.CommitCycleSuccess())

	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRawSelectorRejected && fact.MetricName == "ignored"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelRuleEvaluated &&
			fact.RuleIndex == 0 && fact.RelabelRuleMatched &&
			fact.InputMetricName == "raw_value" && fact.OutputMetricName == "app_value"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelDropped &&
			fact.RuleIndex == 1 && fact.RelabelAction == relabel.Drop && fact.MetricName == "app_value"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelOutput &&
			fact.Source.Family == "raw_value" && fact.Destination.Family == "app_value" &&
			len(fact.OutputLabels) == 1 && fact.OutputLabels[0] == (PipelineLabel{Name: "state", Value: "keep"})
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineWriterFamilyRejected &&
			fact.Reason == PipelineReasonSeriesLimit &&
			fact.MetricName == "app_many"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineWriterSeriesAccepted && fact.Destination.Family == "app_value"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineProfileSelected && fact.ProfileName == "candidate"
	})

	for _, fact := range facts {
		assert.NotContains(t, fact.InputLabelNames, "keep")
		assert.NotContains(t, fact.InputLabelNames, "drop")
	}
}

func TestPipelineDiagnosticsDistinguishJobAndProfileRelabeling(t *testing.T) {
	catalog := loadTestCatalog(t, map[string]string{
		"candidate": testRelabelProfileYAML("app_*", "app_raw", "app_final"),
	})
	dumpPath := filepath.Join(t.TempDir(), "metrics.txt")
	require.NoError(t, os.WriteFile(dumpPath, []byte("# TYPE source_raw gauge\nsource_raw 7\n"), 0o600))

	var facts []PipelineDiagnostic
	collector := NewWithOptions(
		WithProfileCatalog(catalog),
		WithPipelineDiagnosticObserver(func(fact PipelineDiagnostic) { facts = append(facts, fact) }),
	)
	collector.URL = "file://" + dumpPath
	collector.Relabeling = []relabel.Block{{
		Match: "source_*",
		MetricRelabelConfigs: []relabel.Config{{
			SourceLabels: []string{"__name__"},
			Regex:        relabel.MustNewRegexp("source_raw"),
			TargetLabel:  "__name__",
			Replacement:  "app_raw",
			Action:       relabel.Replace,
		}},
	}}
	collector.Profiles = ProfilesConfig{
		Mode:      profilesModeExact,
		ModeExact: &ProfilesModeConfig{Entries: []ProfileEntryConfig{{Name: "candidate"}}},
	}

	ctx := context.Background()
	require.NoError(t, collector.Init(ctx))
	defer collector.Cleanup(ctx)
	require.NoError(t, collector.Check(ctx))

	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelOutput &&
			fact.RelabelStage == PipelineRelabelStageJob && fact.ProfileName == "" &&
			fact.Source.Family == "source_raw" && fact.Destination.Family == "app_raw"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelRuleEvaluated &&
			fact.RelabelStage == PipelineRelabelStageProfile && fact.ProfileName == "candidate" &&
			fact.InputMetricName == "app_raw" && fact.OutputMetricName == "app_final"
	})
	assertPipelineFact(t, facts, func(fact PipelineDiagnostic) bool {
		return fact.Decision == PipelineRelabelOutput &&
			fact.RelabelStage == PipelineRelabelStageProfile && fact.ProfileName == "candidate" &&
			fact.Source.Family == "app_raw" && fact.Destination.Family == "app_final"
	})
}

func assertPipelineFact(t *testing.T, facts []PipelineDiagnostic, matches func(PipelineDiagnostic) bool) {
	t.Helper()
	if slices.ContainsFunc(facts, matches) {
		return
	}
	t.Fatalf("missing pipeline fact in %#v", facts)
}

func collectPipelineSnapshot(t *testing.T, url string, opts ...CollectorOption) map[string]uint64 {
	return collectConfiguredPipelineSnapshot(t, url, nil, opts...)
}

func collectConfiguredPipelineSnapshot(
	t *testing.T,
	url string,
	configure func(*Collector),
	opts ...CollectorOption,
) map[string]uint64 {
	t.Helper()
	collector := NewWithOptions(opts...)
	collector.URL = url
	if configure != nil {
		configure(collector)
	}
	require.NoError(t, collector.Init(context.Background()))
	t.Cleanup(func() { collector.Cleanup(context.Background()) })
	require.NoError(t, collector.Check(context.Background()))

	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())

	got := make(map[string]uint64)
	collector.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()).ForEachSeriesIdentity(
		func(identity metrix.SeriesIdentity, _ metrix.SeriesMeta, _ string, _ metrix.LabelView, value metrix.SampleValue) {
			got[string(identity.ID)] = math.Float64bits(value)
		},
	)
	return got
}
