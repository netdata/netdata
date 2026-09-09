// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"cmp"
	"slices"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
)

type orderedSemanticRelabelOccurrence struct {
	profileOrder int
	stage        promcollector.PipelineRelabelStage
	block        int
	rule         int
	fact         promcollector.PipelineDiagnostic
}

// semanticRelabelOccurrences projects the production execution order. Runtime
// paths remain diagnostic metadata; lexical path ordering is not execution
// ordering once block or rule indexes reach two digits.
func semanticRelabelOccurrences(
	pipeline *pipelineDiagnosticSummary,
	raw prompkg.RawSampleIdentity,
	profiles []promprofiles.Profile,
) []promreplay.SemanticRelabelOccurrence {
	profileOrder := make(map[string]int, len(profiles))
	for index, profile := range profiles {
		profileOrder[profile.Name] = index
	}
	var ordered []orderedSemanticRelabelOccurrence
	for key, fact := range pipeline.rulesForRaw(raw) {
		order := -1
		if key.location.stage == promcollector.PipelineRelabelStageProfile {
			order = profileOrder[key.location.profile]
		}
		ordered = append(ordered, orderedSemanticRelabelOccurrence{
			profileOrder: order,
			stage:        key.location.stage,
			block:        key.location.block,
			rule:         key.rule,
			fact:         fact,
		})
	}
	slices.SortFunc(ordered, func(left, right orderedSemanticRelabelOccurrence) int {
		if order := cmp.Compare(left.profileOrder, right.profileOrder); order != 0 {
			return order
		}
		if order := cmp.Compare(left.block, right.block); order != 0 {
			return order
		}
		return cmp.Compare(left.rule, right.rule)
	})
	out := make([]promreplay.SemanticRelabelOccurrence, 0, len(ordered))
	for _, item := range ordered {
		fact := item.fact
		out = append(out, promreplay.SemanticRelabelOccurrence{
			Profile:          fact.ProfileName,
			RuntimePath:      semanticRelabelRulePath(item.stage, item.block, item.rule),
			Action:           string(fact.RelabelAction),
			Matched:          fact.RelabelRuleMatched,
			Dropped:          fact.RelabelRuleDropped,
			InputMetricName:  fact.InputMetricName,
			OutputMetricName: fact.OutputMetricName,
			InputLabels:      semanticPipelineLabels(fact.InputLabels),
			OutputLabels:     semanticPipelineLabels(fact.OutputLabels),
		})
	}
	return out
}
