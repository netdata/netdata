// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"fmt"
	"maps"
	"slices"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type pipelineBlockSampleKey struct {
	raw   prompkg.RawSampleIdentity
	block int
}

type pipelineDestinationOccurrence struct {
	series prompkg.SampleSeriesIdentity
	value  promcollector.PipelineValueIdentity
	scalar bool
}

type pipelineDiagnosticSummary struct {
	audits              relabelPolicyAudits
	blockInputLabels    map[pipelineBlockSampleKey]map[string]struct{}
	sourcesByFamily     map[string]map[prompkg.SampleSeriesIdentity]struct{}
	writerAccepted      map[pipelineDestinationOccurrence]struct{}
	writerFamilyRejects map[string]promcollector.PipelineReason
	writerSeriesRejects map[pipelineDestinationOccurrence]promcollector.PipelineReason
	selectorRejected    map[prompkg.RawSampleIdentity]struct{}
	typedRejects        map[string]struct{}
	selectedProfiles    map[string]struct{}
}

func newPipelineDiagnosticSummary(policy jobPolicy, batch prompkg.SampleBatch) *pipelineDiagnosticSummary {
	summary := &pipelineDiagnosticSummary{
		audits: relabelPolicyAudits{
			discards:     make(map[relabelDiscardRuleKey]*relabelDiscardAudit),
			nameRewrites: make(map[relabelDiscardRuleKey]*relabelNameRewriteAudit),
			provenance:   make(pipelineProvenance),
			invalidNameDrops: relabelInvalidNameDropAudit{
				blocks:            make(map[int]struct{}),
				families:          make(map[string]struct{}),
				logicalIdentities: make(map[prompkg.SampleSeriesIdentity]struct{}),
				rawSamples:        make(map[prompkg.RawSampleIdentity]struct{}),
			},
		},
		blockInputLabels:    make(map[pipelineBlockSampleKey]map[string]struct{}),
		sourcesByFamily:     make(map[string]map[prompkg.SampleSeriesIdentity]struct{}),
		writerAccepted:      make(map[pipelineDestinationOccurrence]struct{}),
		writerFamilyRejects: make(map[string]promcollector.PipelineReason),
		writerSeriesRejects: make(map[pipelineDestinationOccurrence]promcollector.PipelineReason),
		selectorRejected:    make(map[prompkg.RawSampleIdentity]struct{}),
		typedRejects:        make(map[string]struct{}),
		selectedProfiles:    make(map[string]struct{}),
	}

	for blockIndex, block := range policy.Relabeling {
		for ruleIndex, config := range block.MetricRelabelConfigs {
			if action, ok := sampleDiscardingRelabelAction(config.Action); ok {
				summary.audits.discards[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}] = &relabelDiscardAudit{
					action:            action,
					blockMatch:        block.Match,
					families:          make(map[string]struct{}),
					metricNames:       make(map[string]struct{}),
					logicalIdentities: make(map[prompkg.SampleSeriesIdentity]struct{}),
					rawSamples:        make(map[prompkg.RawSampleIdentity]struct{}),
				}
			}
			if isMetricNameReplace(config) {
				summary.audits.nameRewrites[relabelDiscardRuleKey{block: blockIndex, rule: ruleIndex}] = &relabelNameRewriteAudit{
					metricNames:          make(map[string]struct{}),
					blockInputLabelNames: make(map[string]struct{}),
				}
			}
		}
	}

	for _, sample := range batch.Samples {
		source := prompkg.IdentifySampleSeries(sample)
		identities := summary.sourcesByFamily[source.Family]
		if identities == nil {
			identities = make(map[prompkg.SampleSeriesIdentity]struct{})
			summary.sourcesByFamily[source.Family] = identities
		}
		identities[source] = struct{}{}
	}
	return summary
}

func isMetricNameReplace(config relabel.Config) bool {
	return relabel.EffectiveAction(config) == relabel.Replace &&
		len(config.SourceLabels) == 1 && config.SourceLabels[0] == "__name__" &&
		config.TargetLabel == "__name__"
}

func (s *pipelineDiagnosticSummary) observe(fact promcollector.PipelineDiagnostic) {
	if s == nil {
		return
	}
	switch fact.Decision {
	case promcollector.PipelineRawSelectorRejected:
		s.selectorRejected[fact.RawIdentity] = struct{}{}
	case promcollector.PipelineRelabelBlockEntered:
		key := pipelineBlockSampleKey{raw: fact.RawIdentity, block: fact.BlockIndex}
		labels := s.blockInputLabels[key]
		if labels == nil {
			labels = make(map[string]struct{})
			s.blockInputLabels[key] = labels
		}
		for _, name := range fact.InputLabelNames {
			labels[name] = struct{}{}
		}
	case promcollector.PipelineRelabelRuleEvaluated:
		key := relabelDiscardRuleKey{block: fact.BlockIndex, rule: fact.RuleIndex}
		if audit := s.audits.nameRewrites[key]; audit != nil && fact.RelabelRuleMatched {
			audit.metricNames[fact.InputMetricName] = struct{}{}
			for label := range s.blockInputLabels[pipelineBlockSampleKey{raw: fact.RawIdentity, block: fact.BlockIndex}] {
				audit.blockInputLabelNames[label] = struct{}{}
			}
		}
		if audit := s.audits.discards[key]; audit != nil && fact.RelabelRuleDropped {
			audit.families[fact.Source.Family] = struct{}{}
			audit.metricNames[fact.InputMetricName] = struct{}{}
			audit.logicalIdentities[fact.Source] = struct{}{}
			audit.rawSamples[fact.RawIdentity] = struct{}{}
		}
	case promcollector.PipelineRelabelDropped:
		if fact.RelabelDrop.Reason == relabel.DropReasonInvalidMetricName {
			drops := &s.audits.invalidNameDrops
			drops.blocks[fact.BlockIndex] = struct{}{}
			drops.families[fact.Source.Family] = struct{}{}
			drops.logicalIdentities[fact.Source] = struct{}{}
			drops.rawSamples[fact.RawIdentity] = struct{}{}
		}
	case promcollector.PipelineRelabelOutput:
		destination := pipelineDestinationOccurrence{
			series: fact.Destination,
			value:  fact.ValueIdentity,
			scalar: fact.ScalarValue,
		}
		s.audits.provenance.sourceDestinations(fact.Source)[destination] = struct{}{}
	case promcollector.PipelineTypedFamilyRejected:
		s.typedRejects[fact.MetricName] = struct{}{}
	case promcollector.PipelineWriterFamilyRejected:
		s.writerFamilyRejects[fact.MetricName] = fact.Reason
	case promcollector.PipelineWriterSeriesRejected:
		destination := pipelineDestinationOccurrence{series: fact.Destination, value: fact.ValueIdentity, scalar: fact.ScalarValue}
		s.writerSeriesRejects[destination] = fact.Reason
	case promcollector.PipelineWriterSeriesAccepted:
		destination := pipelineDestinationOccurrence{series: fact.Destination, value: fact.ValueIdentity, scalar: fact.ScalarValue}
		s.writerAccepted[destination] = struct{}{}
	case promcollector.PipelineProfileSelected:
		s.selectedProfiles[fact.ProfileName] = struct{}{}
	}
}

func (s *pipelineDiagnosticSummary) finalize() relabelPolicyAudits {
	if s == nil {
		return relabelPolicyAudits{}
	}
	for _, audit := range s.audits.discards {
		audit.rawSeries = len(audit.rawSamples)
	}
	s.audits.invalidNameDrops.rawSeries = len(s.audits.invalidNameDrops.rawSamples)

	sourcesByDestination := make(map[prompkg.SampleSeriesIdentity]map[prompkg.SampleSeriesIdentity]struct{})
	for source, destinations := range s.audits.provenance {
		for destination := range destinations {
			if _, accepted := s.writerAccepted[destination]; !accepted {
				continue
			}
			sources := sourcesByDestination[destination.series]
			if sources == nil {
				sources = make(map[prompkg.SampleSeriesIdentity]struct{})
				sourcesByDestination[destination.series] = sources
			}
			sources[source] = struct{}{}
		}
	}

	collapsedSources := make(map[prompkg.SampleSeriesIdentity]struct{})
	collapsedFamilies := make(map[string]struct{})
	for destination, sources := range sourcesByDestination {
		if len(sources) < 2 {
			continue
		}
		s.audits.identityCollapse.finalIdentities++
		collapsedFamilies[destination.Family] = struct{}{}
		for source := range sources {
			collapsedSources[source] = struct{}{}
		}
	}
	s.audits.identityCollapse.finalFamilies = slices.Sorted(maps.Keys(collapsedFamilies))
	s.audits.identityCollapse.sourceIdentities = len(collapsedSources)
	return s.audits
}

func addRelabelPolicyFindings(audits relabelPolicyAudits, r *report) {
	keys := slices.SortedFunc(maps.Keys(audits.discards), func(a, b relabelDiscardRuleKey) int {
		if a.block != b.block {
			return a.block - b.block
		}
		return a.rule - b.rule
	})
	for _, key := range keys {
		audit := audits.discards[key]
		path := fmt.Sprintf("relabeling[%d].metric_relabel_configs[%d]", key.block, key.rule)
		message := fmt.Sprintf(
			"sample-discarding relabel action %q in block match %q dropped no samples in the supplied dump",
			audit.action,
			audit.blockMatch,
		)
		if audit.rawSeries > 0 {
			message = fmt.Sprintf(
				"sample-discarding relabel action %q in block match %q removes %d observed logical identities across %d source families (%d raw exposition series)",
				audit.action,
				audit.blockMatch,
				len(audit.logicalIdentities),
				len(audit.families),
				audit.rawSeries,
			)
		}
		r.addWarning(
			"job_relabel_discard_review",
			path,
			message,
			"Drop, keep, dropequal, and keepequal change the evidence denominator before profile coverage is measured. Confirm authoritative semantics and state which operator question is lost; zero observed drops do not prove the rule is harmless for unseen values.",
		)
	}
}
