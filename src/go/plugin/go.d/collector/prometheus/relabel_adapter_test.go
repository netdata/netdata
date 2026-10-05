// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"testing"

	commonmodel "github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/relabel"
)

// Relabeling owns only names and labels. The collector must preserve the parsed
// payload, including when a later block drops an already transformed record.
func TestCollector_relabelPreservesPayload(t *testing.T) {
	rename := relabel.Block{
		Match: "*",
		MetricRelabelConfigs: []relabel.Config{
			{TargetLabel: "__name__", Replacement: "renamed", Action: relabel.Replace},
			{TargetLabel: "stage", Replacement: "first", Action: relabel.Replace},
		},
	}
	drop := relabel.Block{
		Match: "*",
		MetricRelabelConfigs: []relabel.Config{
			{TargetLabel: "__name__", Replacement: "discarded", Action: relabel.Replace},
			{TargetLabel: "stage", Replacement: "discarded", Action: relabel.Replace},
			{Action: relabel.Drop},
		},
	}
	scenarios := map[string]struct {
		blocks  []relabel.Block
		renamed bool
		dropped bool
	}{
		"no pipeline": {},
		"unmatched block": {
			blocks: []relabel.Block{{Match: "other_*", MetricRelabelConfigs: drop.MetricRelabelConfigs}},
		},
		"rewrite":          {blocks: []relabel.Block{rename}, renamed: true},
		"drop first block": {blocks: []relabel.Block{drop}, dropped: true},
		"drop later block": {blocks: []relabel.Block{rename, drop}, renamed: true, dropped: true},
	}
	payloads := map[string]struct {
		kind       prompkg.SampleKind
		familyType commonmodel.MetricType
	}{
		"gauge":           {prompkg.SampleKindScalar, commonmodel.MetricTypeGauge},
		"counter":         {prompkg.SampleKindScalar, commonmodel.MetricTypeCounter},
		"bucket":          {prompkg.SampleKindHistogramBucket, commonmodel.MetricTypeHistogram},
		"histogram sum":   {prompkg.SampleKindHistogramSum, commonmodel.MetricTypeHistogram},
		"histogram count": {prompkg.SampleKindHistogramCount, commonmodel.MetricTypeHistogram},
		"quantile":        {prompkg.SampleKindSummaryQuantile, commonmodel.MetricTypeSummary},
		"summary sum":     {prompkg.SampleKindSummarySum, commonmodel.MetricTypeSummary},
		"summary count":   {prompkg.SampleKindSummaryCount, commonmodel.MetricTypeSummary},
	}
	for scenarioName, scenario := range scenarios {
		t.Run(scenarioName, func(t *testing.T) {
			for payloadName, payload := range payloads {
				t.Run(payloadName, func(t *testing.T) {
					for mode, observed := range map[string]bool{"plain": false, "observed": true} {
						t.Run(mode, func(t *testing.T) {
							pipeline, err := relabel.NewPipeline(scenario.blocks)
							require.NoError(t, err)
							var facts []PipelineDiagnostic
							var observer PipelineDiagnosticObserver
							if observed {
								observer = func(fact PipelineDiagnostic) { facts = append(facts, fact) }
							}
							c := NewWithOptions(WithPipelineDiagnosticObserver(observer))
							raw := prompkg.Sample{
								Name:       "original",
								Labels:     labels.FromStrings("instance", "one"),
								Value:      13.25,
								Kind:       payload.kind,
								FamilyType: payload.familyType,
							}
							want := raw
							if scenario.renamed {
								want.Name = "renamed"
								want.Labels = labels.FromStrings("instance", "one", "stage", "first")
							}
							got, dropped := c.applyObservedPipeline(raw, pipeline, jobRelabelStage, "", true)
							assert.Equal(t, want, got)
							assert.Equal(t, scenario.dropped, dropped.Dropped())
							assert.Equal(t, labels.FromStrings("instance", "one"), raw.Labels)
							if observed {
								require.NotEmpty(t, facts)
								if scenario.dropped {
									assert.Equal(t, PipelineRelabelDropped, facts[len(facts)-1].Decision)
									assert.Equal(t, "discarded", facts[len(facts)-1].MetricName)
								} else {
									assert.Equal(t, PipelineRelabelOutput, facts[len(facts)-1].Decision)
									assert.Equal(t, want.Name, facts[len(facts)-1].MetricName)
								}
							}
						})
					}
				})
			}
		})
	}
}
