// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"slices"
	"testing"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

func TestSemanticRelabelOccurrencesPreserveNumericExecutionOrder(t *testing.T) {
	raw := prompkg.RawSampleIdentity{1}
	node := pipelinePhysicalOccurrence{raw: raw}
	summary := &pipelineDiagnosticSummary{
		initialNodes: map[prompkg.RawSampleIdentity]pipelinePhysicalOccurrence{raw: node},
		rulesEvaluated: map[pipelinePhysicalOccurrence]map[pipelineRuleKey]promcollector.PipelineDiagnostic{
			node: {
				{location: pipelineRelabelLocation{stage: promcollector.PipelineRelabelStageProfile, profile: "candidate", block: 10}, rule: 0}: {
					RelabelStage: promcollector.PipelineRelabelStageProfile, ProfileName: "candidate",
				},
				{location: pipelineRelabelLocation{stage: promcollector.PipelineRelabelStageJob, block: 3}, rule: 0}: {
					RelabelStage: promcollector.PipelineRelabelStageJob,
				},
				{location: pipelineRelabelLocation{stage: promcollector.PipelineRelabelStageProfile, profile: "candidate", block: 2}, rule: 1}: {
					RelabelStage: promcollector.PipelineRelabelStageProfile, ProfileName: "candidate",
				},
			},
		},
	}
	got := semanticRelabelOccurrences(
		summary,
		raw,
		[]promprofiles.Profile{{Name: "candidate"}},
	)
	paths := make([]string, 0, len(got))
	for _, occurrence := range got {
		paths = append(paths, occurrence.RuntimePath)
	}
	want := []string{
		"job.relabeling[3].metric_relabel_configs[0]",
		"relabeling[2].metric_relabel_configs[1]",
		"relabeling[10].metric_relabel_configs[0]",
	}
	if !slices.Equal(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
}
