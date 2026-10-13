// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	promlabels "github.com/prometheus/prometheus/model/labels"
)

type writerSnapshot map[metrix.SeriesID]float64

func snapshotWriter(reader metrix.Reader) writerSnapshot {
	snapshot := make(writerSnapshot)
	reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, _ metrix.SeriesMeta, _ string, _ metrix.LabelView, value metrix.SampleValue) {
		snapshot[identity.ID] = value
	})
	return snapshot
}

func destinationSeriesIDs(reader metrix.Reader) map[prompkg.SampleSeriesIdentity]metrix.SeriesID {
	ids := make(map[prompkg.SampleSeriesIdentity]metrix.SeriesID)
	reader.ForEachSeriesIdentity(func(identity metrix.SeriesIdentity, _ metrix.SeriesMeta, name string, labelView metrix.LabelView, _ metrix.SampleValue) {
		var lbs promlabels.Labels
		labelView.Range(func(key, value string) bool {
			lbs = append(lbs, promlabels.Label{Name: key, Value: value})
			return true
		})
		slices.SortFunc(lbs, func(a, b promlabels.Label) int { return strings.Compare(a.Name, b.Name) })
		ids[prompkg.IdentifySeries(name, lbs, "")] = identity.ID
	})
	return ids
}

type futureProbeState struct {
	index       int
	input       futureInput
	rawIdentity prompkg.RawSampleIdentity
	finalNames  []string
	seriesIDs   []metrix.SeriesID
	open        bool
}

func probeAllowsAuthoredRouting(
	requirements futureRequirements,
	pipeline *pipelineDiagnosticSummary,
	rawIdentity prompkg.RawSampleIdentity,
) bool {
	for _, requirement := range requirements.rules {
		if !requirement.allowsAuthoredRouting {
			continue
		}
		fact, evaluated := pipeline.ruleForRaw(rawIdentity, pipelineRuleKey{
			location: requirement.location, rule: requirement.ruleIndex,
		})
		if evaluated && fact.RelabelRuleMatched {
			return true
		}
	}
	return false
}

func inspectFutureOpenness(
	inputs []futureInput,
	requirements futureRequirements,
	pipeline *pipelineDiagnosticSummary,
	routes *planRouteSummary,
	current writerSnapshot,
	futureReader metrix.Reader,
	r *Report,
) map[string]string {
	futureSnapshot := snapshotWriter(futureReader)
	for id, value := range current {
		futureValue, ok := futureSnapshot[id]
		if !ok || !sameWriterValue(futureValue, value) {
			r.addError(
				"future_run_changed_current_evidence",
				"future_inputs",
				fmt.Sprintf("future run changed or removed current writer series %q", id),
				"The future-openness sequence is isolated from current coverage, but adding probes must not mutate, evict, or overwrite current evidence identities.",
			)
		}
	}

	seriesIDs := destinationSeriesIDs(futureReader)
	probes := make([]futureProbeState, 0, len(inputs))
	firstOwnedProbe := make(map[string]int)
	sourcesByDestination := make(map[prompkg.SampleSeriesIdentity][]int)
	for index, input := range inputs {
		rawIdentity := prompkg.IdentifyRawSample(input.Name, promlabels.FromMap(input.Labels))
		probe := futureProbeState{index: index, input: input, rawIdentity: rawIdentity}
		path := fmt.Sprintf("future_inputs[%d]", index)

		_, rawAccepted := pipeline.rawAccepted[rawIdentity]
		if _, rejected := pipeline.selectorRejected[rawIdentity]; rejected {
			r.addError(
				"future_metric_blocked_by_job_selector", path,
				fmt.Sprintf("raw future metric %q is rejected by the job selector", input.Name),
				"Every future probe must enter the real relabel, assembly, writer, profile, and fallback pipeline.",
			)
		} else if !rawAccepted {
			r.addError(
				"future_metric_missing_from_pipeline", path,
				fmt.Sprintf("raw future metric %q produced no selector decision", input.Name),
				"The staged exposition and production parser must preserve every declared or derived raw probe.",
			)
		}
		drop, dropped := pipeline.relabelDropForRaw(rawIdentity)
		if dropped {
			key := pipelineRuleKey{
				location: pipelineRelabelLocation{
					stage:   drop.RelabelStage,
					profile: drop.ProfileName,
					block:   drop.BlockIndex,
				},
				rule: drop.RuleIndex,
			}
			_, boundedExclusion := requirements.boundedDropRules[key]
			owner := "job"
			code := "future_metric_blocked_by_job_relabel"
			if drop.RelabelStage == promcollector.PipelineRelabelStageProfile {
				owner = fmt.Sprintf("profile %q", drop.ProfileName)
				code = "future_metric_blocked_by_profile_relabel"
			}
			if !boundedExclusion {
				r.addError(
					code, path,
					fmt.Sprintf("raw future metric %q is dropped by %s relabeling block %d rule %d (%s)", input.Name, owner, drop.BlockIndex, drop.RuleIndex, drop.RelabelDrop.Reason),
					"Recommended relabeling must leave unknown future exporter metrics open unless the exclusion is exact, bounded, and current-source-proven.",
				)
			}
		}

		destinations := pipeline.finalDestinationsForRaw(rawIdentity)
		if rawAccepted && !dropped && len(destinations) == 0 {
			r.addError(
				"future_metric_missing_after_relabel", path,
				fmt.Sprintf("raw future metric %q produced no relabel output identity", input.Name),
				"A probe accepted by the raw selector must either produce a production relabel destination or an explicit relabel-drop fact.",
			)
		}
		for destination := range destinations {
			if _, accepted := pipeline.writerAccepted[destination]; !accepted {
				reason := pipeline.writerSeriesRejects[destination]
				if reason == "" {
					reason = pipeline.writerFamilyRejects[destination.series.Family]
				}
				r.addError(
					"future_metric_rejected_by_writer", path,
					fmt.Sprintf("future metric %q reaches writer destination %q but is rejected (%s)", input.Name, destination.series.Family, reason),
					"A future witness proves openness only when the production writer commits its final identity and value.",
				)
				continue
			}
			probe.finalNames = append(probe.finalNames, destination.series.Family)
			sourcesByDestination[destination.series] = append(sourcesByDestination[destination.series], index)
			seriesID, ok := seriesIDs[destination.series]
			if !ok {
				r.addError(
					"future_metric_missing_from_writer", path,
					fmt.Sprintf("accepted future destination %q is absent from the committed reader", destination.series.Family),
					"Pipeline diagnostics and committed metrix evidence must describe the same production writer result.",
				)
				continue
			}
			probe.seriesIDs = append(probe.seriesIDs, seriesID)
			if _, existed := current[seriesID]; existed {
				r.addError(
					"future_metric_identity_collapse", path,
					fmt.Sprintf("future metric %q collapses onto current writer series %q", input.Name, seriesID),
					"A future raw family must retain a distinct final writer identity instead of overwriting current evidence.",
				)
			}
		}
		slices.Sort(probe.finalNames)
		probe.finalNames = slices.Compact(probe.finalNames)
		slices.Sort(probe.seriesIDs)
		probe.seriesIDs = slices.Compact(probe.seriesIDs)

		if len(probe.seriesIDs) > 0 {
			probe.open = true
			allowsAuthoredRouting := probeAllowsAuthoredRouting(requirements, pipeline, rawIdentity)
			for _, id := range probe.seriesIDs {
				route := routes.series[id]
				switch {
				case route == nil:
					probe.open = false
					r.addError(
						"future_metric_missing_from_planner", path,
						fmt.Sprintf("future writer series %q produced no chartengine route fact", id),
						"A future witness must traverse the production chart planner as well as the collector.",
					)
				case route.unmatched:
					probe.open = false
					r.addError(
						"future_metric_blocked_by_profile", path,
						fmt.Sprintf("future writer series %q is unmatched by chart routing (%s)", id, route.unmatchedReason),
						"Unknown future families in the profile namespace must remain eligible for generic fallback charts.",
					)
				case !route.autogen && !allowsAuthoredRouting:
					probe.open = false
					r.addError(
						"future_metric_routed_to_authored_metric", path,
						fmt.Sprintf("future writer series %q routes to authored charts instead of generic fallback", id),
						"A name rewrite must not map unknown future families onto an authored metric contract.",
					)
				}
			}
		}
		probes = append(probes, probe)
	}

	for destination, sources := range sourcesByDestination {
		if len(sources) < 2 {
			continue
		}
		slices.Sort(sources)
		r.addError(
			"future_metric_identity_collapse",
			"future_inputs",
			fmt.Sprintf("future inputs %v collapse onto writer destination %q", sources, destination.Family),
			"Distinct raw future identities must remain distinct after relabeling and writer normalization.",
		)
	}

	for _, scope := range requirements.profileScopes {
		matched := false
		scopeMatcher, err := matcher.NewSimplePatternsMatcher(scope.scopeExpr)
		if err != nil {
			r.addError("future_metric_analysis", scope.path, err.Error(), "Profile wildcard coverage must use the production matcher grammar.")
			continue
		}
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			if scopeMatcher.MatchString(probe.input.Name) {
				matched = true
				recordFirstOwnedProbe(firstOwnedProbe, scope.owner, probe.index)
				break
			}
		}
		if !matched {
			r.addError(
				"future_profile_term_uncovered", scope.path,
				fmt.Sprintf("no open future probe reaches positive wildcard profile term %q", scope.pattern),
				"Each positive wildcard profile namespace requires a raw probe that survives selector, relabeling, writer, and generic chart routing.",
			)
		}
	}

	for _, scope := range requirements.blockScopes {
		matched := false
		scopeMatcher, err := matcher.NewSimplePatternsMatcher(scope.scopeExpr)
		if err != nil {
			r.addError("future_metric_analysis", scope.path, err.Error(), "Relabel wildcard coverage must use the production matcher grammar.")
			continue
		}
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			entry, entered := pipeline.blockEntryForRaw(probe.rawIdentity, scope.location)
			if entered && scopeMatcher.MatchString(entry) {
				matched = true
				recordFirstOwnedProbe(firstOwnedProbe, scope.owner, probe.index)
				break
			}
		}
		if !matched {
			r.addError(
				"future_relabel_scope_uncovered", scope.path,
				fmt.Sprintf("no open future probe enters relabel wildcard term %q", scope.pattern),
				"Every reachable wildcard relabel scope that can rename or drop samples needs a surviving raw future witness.",
			)
		}
	}

	for _, requirement := range requirements.rules {
		covered := false
		for _, probe := range probes {
			if !probe.open {
				continue
			}
			fact, evaluated := pipeline.ruleForRaw(probe.rawIdentity, pipelineRuleKey{
				location: requirement.location, rule: requirement.ruleIndex,
			})
			if evaluated && (!requirement.requireHit || fact.RelabelRuleMatched) {
				covered = true
				recordFirstOwnedProbe(firstOwnedProbe, requirement.owner, probe.index)
				break
			}
		}
		if !covered {
			r.addError(
				"future_relabel_branch_uncovered", requirement.path,
				"no open future probe exercises this reachable rename/drop-capable relabel rule",
				"Declare enough raw future inputs and labels to cover every reachable routing-affecting relabel branch; name-writing rules must take their matching branch.",
			)
		}
	}
	result := make(map[string]string, len(firstOwnedProbe))
	for owner, index := range firstOwnedProbe {
		result[owner] = inputs[index].Name
	}
	return result
}

func recordFirstOwnedProbe(first map[string]int, owner string, index int) {
	if owner == "" {
		return
	}
	if previous, ok := first[owner]; !ok || index < previous {
		first[owner] = index
	}
}

func sameWriterValue(left, right metrix.SampleValue) bool {
	return left == right || (math.IsNaN(left) && math.IsNaN(right))
}
