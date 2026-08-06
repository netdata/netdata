// SPDX-License-Identifier: GPL-3.0-or-later

package promprofilevalidation

import (
	"fmt"
	"maps"
	"slices"

	prompkg "github.com/netdata/netdata/go/plugins/pkg/prometheus"
	promcollector "github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/promprofiles"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/prometheus/relabel"
)

type pipelineRelabelLocation struct {
	stage   promcollector.PipelineRelabelStage
	profile string
	block   int
}

type validationRelabelStage struct {
	stage   promcollector.PipelineRelabelStage
	profile string
	blocks  []relabel.Block
	offset  int
}

func validationRelabelStages(
	policy jobPolicy,
	profiles []promprofiles.Profile,
) ([]validationRelabelStage, error) {
	stages := []validationRelabelStage{{
		stage:  promcollector.PipelineRelabelStageJob,
		blocks: policy.Relabeling,
	}}
	offset := len(policy.Relabeling)
	for _, profile := range profiles {
		profileBlocks, err := profile.Relabeling()
		if err != nil {
			return nil, fmt.Errorf("profile %q relabeling: %w", profile.Name, err)
		}
		stages = append(stages, validationRelabelStage{
			stage:   promcollector.PipelineRelabelStageProfile,
			profile: profile.Name,
			blocks:  profileBlocks,
			offset:  offset,
		})
		offset += len(profileBlocks)
	}
	return stages, nil
}

type pipelineBlockSampleKey struct {
	occurrence pipelineDestinationOccurrence
	location   pipelineRelabelLocation
}

type pipelineDestinationOccurrence struct {
	series prompkg.SampleSeriesIdentity
	value  promcollector.PipelineValueIdentity
	scalar bool
}

type pipelineRuleKey struct {
	location pipelineRelabelLocation
	rule     int
}

type pipelineDiagnosticSummary struct {
	audits               relabelPolicyAudits
	blockInputLabels     map[pipelineBlockSampleKey]map[string]struct{}
	sourcesByFamily      map[string]map[prompkg.SampleSeriesIdentity]struct{}
	writerAccepted       map[pipelineDestinationOccurrence]struct{}
	writerFamilyRejects  map[string]promcollector.PipelineReason
	writerSeriesRejects  map[pipelineDestinationOccurrence]promcollector.PipelineReason
	selectorRejected     map[prompkg.RawSampleIdentity]struct{}
	rawAccepted          map[prompkg.RawSampleIdentity]struct{}
	originsByRaw         map[prompkg.RawSampleIdentity]pipelineDestinationOccurrence
	originsBySource      map[prompkg.SampleSeriesIdentity]map[pipelineDestinationOccurrence]struct{}
	relabelDropped       map[pipelineDestinationOccurrence]map[pipelineRelabelLocation]promcollector.PipelineDiagnostic
	blockEntries         map[pipelineDestinationOccurrence]map[pipelineRelabelLocation]string
	rulesEvaluated       map[pipelineDestinationOccurrence]map[pipelineRuleKey]promcollector.PipelineDiagnostic
	destinations         map[pipelineDestinationOccurrence]map[pipelineDestinationOccurrence]struct{}
	typedRejects         map[string]struct{}
	selectedProfiles     map[string]struct{}
	profileRelabelBlocks int
}

func newPipelineDiagnosticSummary(
	policy jobPolicy,
	profiles []promprofiles.Profile,
	batch prompkg.SampleBatch,
) (*pipelineDiagnosticSummary, error) {
	stages, err := validationRelabelStages(policy, profiles)
	if err != nil {
		return nil, err
	}
	summary := &pipelineDiagnosticSummary{
		audits: relabelPolicyAudits{
			discards:     make(map[relabelDiscardRuleKey]*relabelDiscardAudit),
			nameRewrites: make(map[relabelDiscardRuleKey]*relabelNameRewriteAudit),
			blockInputs:  make(map[pipelineRelabelLocation]map[string]struct{}),
			provenance:   make(pipelineProvenance),
			invalidNameDrops: relabelInvalidNameDropAudit{
				blocks:            make(map[pipelineRelabelLocation]struct{}),
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
		rawAccepted:         make(map[prompkg.RawSampleIdentity]struct{}),
		originsByRaw:        make(map[prompkg.RawSampleIdentity]pipelineDestinationOccurrence),
		originsBySource:     make(map[prompkg.SampleSeriesIdentity]map[pipelineDestinationOccurrence]struct{}),
		relabelDropped:      make(map[pipelineDestinationOccurrence]map[pipelineRelabelLocation]promcollector.PipelineDiagnostic),
		blockEntries:        make(map[pipelineDestinationOccurrence]map[pipelineRelabelLocation]string),
		rulesEvaluated:      make(map[pipelineDestinationOccurrence]map[pipelineRuleKey]promcollector.PipelineDiagnostic),
		destinations:        make(map[pipelineDestinationOccurrence]map[pipelineDestinationOccurrence]struct{}),
		typedRejects:        make(map[string]struct{}),
		selectedProfiles:    make(map[string]struct{}),
	}
	for _, stage := range stages {
		if stage.stage == promcollector.PipelineRelabelStageProfile {
			summary.profileRelabelBlocks += len(stage.blocks)
		}
	}

	addAudits := func(stage promcollector.PipelineRelabelStage, profileName string, blocks []relabel.Block) {
		for blockIndex, block := range blocks {
			for ruleIndex, config := range block.MetricRelabelConfigs {
				key := relabelDiscardRuleKey{stage: stage, profile: profileName, block: blockIndex, rule: ruleIndex}
				if action, ok := sampleDiscardingRelabelAction(config.Action); ok {
					summary.audits.discards[key] = &relabelDiscardAudit{
						action:            action,
						blockMatch:        block.Match,
						families:          make(map[string]struct{}),
						metricNames:       make(map[string]struct{}),
						logicalIdentities: make(map[prompkg.SampleSeriesIdentity]struct{}),
						rawSamples:        make(map[prompkg.RawSampleIdentity]struct{}),
					}
				}
				if isMetricNameReplace(config) {
					summary.audits.nameRewrites[key] = &relabelNameRewriteAudit{
						metricNames:          make(map[string]struct{}),
						blockInputLabelNames: make(map[string]struct{}),
					}
				}
			}
		}
	}
	for _, stage := range stages {
		addAudits(stage.stage, stage.profile, stage.blocks)
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
	return summary, nil
}

func isMetricNameReplace(config relabel.Config) bool {
	return relabel.EffectiveAction(config) == relabel.Replace &&
		len(config.SourceLabels) == 1 && config.SourceLabels[0] == "__name__" &&
		config.TargetLabel == "__name__"
}

func diagnosticOccurrence(fact promcollector.PipelineDiagnostic) pipelineDestinationOccurrence {
	return pipelineDestinationOccurrence{series: fact.Source, value: fact.ValueIdentity, scalar: fact.ScalarValue}
}

func diagnosticLocation(fact promcollector.PipelineDiagnostic) pipelineRelabelLocation {
	return pipelineRelabelLocation{stage: fact.RelabelStage, profile: fact.ProfileName, block: fact.BlockIndex}
}

func (s *pipelineDiagnosticSummary) observe(fact promcollector.PipelineDiagnostic) {
	if s == nil {
		return
	}
	switch fact.Decision {
	case promcollector.PipelineRawSelectorRejected:
		s.selectorRejected[fact.RawIdentity] = struct{}{}
	case promcollector.PipelineRawAccepted:
		s.rawAccepted[fact.RawIdentity] = struct{}{}
		occurrence := diagnosticOccurrence(fact)
		s.originsByRaw[fact.RawIdentity] = occurrence
		origins := s.originsBySource[fact.Source]
		if origins == nil {
			origins = make(map[pipelineDestinationOccurrence]struct{})
			s.originsBySource[fact.Source] = origins
		}
		origins[occurrence] = struct{}{}
	case promcollector.PipelineRelabelBlockEntered:
		occurrence := diagnosticOccurrence(fact)
		location := diagnosticLocation(fact)
		entries := s.blockEntries[occurrence]
		if entries == nil {
			entries = make(map[pipelineRelabelLocation]string)
			s.blockEntries[occurrence] = entries
		}
		entries[location] = fact.InputMetricName
		inputs := s.audits.blockInputs[location]
		if inputs == nil {
			inputs = make(map[string]struct{})
			s.audits.blockInputs[location] = inputs
		}
		inputs[fact.InputMetricName] = struct{}{}
		key := pipelineBlockSampleKey{occurrence: occurrence, location: location}
		labels := s.blockInputLabels[key]
		if labels == nil {
			labels = make(map[string]struct{})
			s.blockInputLabels[key] = labels
		}
		for _, name := range fact.InputLabelNames {
			labels[name] = struct{}{}
		}
	case promcollector.PipelineRelabelRuleEvaluated:
		occurrence := diagnosticOccurrence(fact)
		location := diagnosticLocation(fact)
		rules := s.rulesEvaluated[occurrence]
		if rules == nil {
			rules = make(map[pipelineRuleKey]promcollector.PipelineDiagnostic)
			s.rulesEvaluated[occurrence] = rules
		}
		rules[pipelineRuleKey{location: location, rule: fact.RuleIndex}] = fact
		key := relabelDiscardRuleKey{
			stage: fact.RelabelStage, profile: fact.ProfileName, block: fact.BlockIndex, rule: fact.RuleIndex,
		}
		if audit := s.audits.nameRewrites[key]; audit != nil && fact.RelabelRuleMatched {
			audit.metricNames[fact.InputMetricName] = struct{}{}
			for label := range s.blockInputLabels[pipelineBlockSampleKey{occurrence: occurrence, location: location}] {
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
		occurrence := diagnosticOccurrence(fact)
		location := diagnosticLocation(fact)
		drops := s.relabelDropped[occurrence]
		if drops == nil {
			drops = make(map[pipelineRelabelLocation]promcollector.PipelineDiagnostic)
			s.relabelDropped[occurrence] = drops
		}
		drops[location] = fact
		if fact.RelabelDrop.Reason == relabel.DropReasonInvalidMetricName {
			drops := &s.audits.invalidNameDrops
			drops.blocks[location] = struct{}{}
			drops.families[fact.Source.Family] = struct{}{}
			drops.logicalIdentities[fact.Source] = struct{}{}
			drops.rawSamples[fact.RawIdentity] = struct{}{}
		}
	case promcollector.PipelineRelabelOutput:
		source := diagnosticOccurrence(fact)
		destination := pipelineDestinationOccurrence{
			series: fact.Destination,
			value:  fact.ValueIdentity,
			scalar: fact.ScalarValue,
		}
		destinations := s.destinations[source]
		if destinations == nil {
			destinations = make(map[pipelineDestinationOccurrence]struct{})
			s.destinations[source] = destinations
		}
		destinations[destination] = struct{}{}
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

func (s *pipelineDiagnosticSummary) reachableFromRaw(raw prompkg.RawSampleIdentity) map[pipelineDestinationOccurrence]struct{} {
	reachable := make(map[pipelineDestinationOccurrence]struct{})
	start, ok := s.originsByRaw[raw]
	if !ok {
		return reachable
	}
	queue := []pipelineDestinationOccurrence{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, seen := reachable[current]; seen {
			continue
		}
		reachable[current] = struct{}{}
		for destination := range s.destinations[current] {
			queue = append(queue, destination)
		}
	}
	return reachable
}

func (s *pipelineDiagnosticSummary) relabelDropForRaw(raw prompkg.RawSampleIdentity) (promcollector.PipelineDiagnostic, bool) {
	for occurrence := range s.reachableFromRaw(raw) {
		for _, fact := range s.relabelDropped[occurrence] {
			return fact, true
		}
	}
	return promcollector.PipelineDiagnostic{}, false
}

func (s *pipelineDiagnosticSummary) blockEntryForRaw(
	raw prompkg.RawSampleIdentity,
	location pipelineRelabelLocation,
) (string, bool) {
	for occurrence := range s.reachableFromRaw(raw) {
		if entry, ok := s.blockEntries[occurrence][location]; ok {
			return entry, true
		}
	}
	return "", false
}

func (s *pipelineDiagnosticSummary) ruleForRaw(
	raw prompkg.RawSampleIdentity,
	key pipelineRuleKey,
) (promcollector.PipelineDiagnostic, bool) {
	for occurrence := range s.reachableFromRaw(raw) {
		if fact, ok := s.rulesEvaluated[occurrence][key]; ok {
			return fact, true
		}
	}
	return promcollector.PipelineDiagnostic{}, false
}

func (s *pipelineDiagnosticSummary) finalDestinationsForRaw(
	raw prompkg.RawSampleIdentity,
) map[pipelineDestinationOccurrence]struct{} {
	final := make(map[pipelineDestinationOccurrence]struct{})
	for occurrence := range s.reachableFromRaw(raw) {
		if _, accepted := s.writerAccepted[occurrence]; accepted {
			final[occurrence] = struct{}{}
		}
		if _, rejected := s.writerSeriesRejects[occurrence]; rejected {
			final[occurrence] = struct{}{}
		}
		if _, rejected := s.writerFamilyRejects[occurrence.series.Family]; rejected {
			final[occurrence] = struct{}{}
		}
	}
	return final
}

func (s *pipelineDiagnosticSummary) finalize() relabelPolicyAudits {
	if s == nil {
		return relabelPolicyAudits{}
	}
	for _, audit := range s.audits.discards {
		audit.rawSeries = len(audit.rawSamples)
	}
	s.audits.invalidNameDrops.rawSeries = len(s.audits.invalidNameDrops.rawSamples)
	for source, origins := range s.originsBySource {
		for origin := range origins {
			queue := []pipelineDestinationOccurrence{origin}
			seen := make(map[pipelineDestinationOccurrence]struct{})
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				if _, ok := seen[current]; ok {
					continue
				}
				seen[current] = struct{}{}
				if _, accepted := s.writerAccepted[current]; accepted {
					s.audits.provenance.sourceDestinations(source)[current] = struct{}{}
				}
				for destination := range s.destinations[current] {
					queue = append(queue, destination)
				}
			}
		}
	}

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

func addRelabelPolicyFindings(audits relabelPolicyAudits, r *Report) {
	keys := slices.SortedFunc(maps.Keys(audits.discards), func(a, b relabelDiscardRuleKey) int {
		if a.stage != b.stage {
			return cmpString(string(a.stage), string(b.stage))
		}
		if a.profile != b.profile {
			return cmpString(a.profile, b.profile)
		}
		if a.block != b.block {
			return a.block - b.block
		}
		return a.rule - b.rule
	})
	for _, key := range keys {
		audit := audits.discards[key]
		path := relabelRulePath(key.stage, key.block, key.rule)
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
		code := "job_relabel_discard_review"
		if key.stage == promcollector.PipelineRelabelStageProfile {
			code = "profile_relabel_discard_review"
		}
		r.addWarning(
			code,
			path,
			message,
			"Drop, keep, dropequal, and keepequal change the evidence denominator before profile coverage is measured. Confirm authoritative semantics and state which operator question is lost; zero observed drops do not prove the rule is harmless for unseen values.",
		)
	}
}

func relabelBlockPath(stage promcollector.PipelineRelabelStage, block int) string {
	prefix := "relabeling"
	if stage == promcollector.PipelineRelabelStageProfile {
		prefix = "profile.relabeling"
	}
	return fmt.Sprintf("%s[%d]", prefix, block)
}

func relabelRulePath(stage promcollector.PipelineRelabelStage, block, rule int) string {
	return fmt.Sprintf("%s.metric_relabel_configs[%d]", relabelBlockPath(stage, block), rule)
}

func cmpString(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
