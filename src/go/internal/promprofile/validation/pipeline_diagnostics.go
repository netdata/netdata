// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

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
	raw      prompkg.RawSampleIdentity
	location pipelineRelabelLocation
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

type pipelineStageKey struct {
	stage   promcollector.PipelineRelabelStage
	profile string
}

type pipelinePhysicalOccurrence struct {
	raw        prompkg.RawSampleIdentity
	occurrence pipelineDestinationOccurrence
}

type pipelineTransition struct {
	fact    promcollector.PipelineDiagnostic
	output  pipelinePhysicalOccurrence
	dropped bool
}

type orderedPipelineTransition struct {
	order      int
	transition pipelineTransition
}

type pipelineDiagnosticSummary struct {
	audits               relabelPolicyAudits
	blockInputLabels     map[pipelineBlockSampleKey]map[string]struct{}
	sourcesByFamily      map[string]map[prompkg.SampleSeriesIdentity]struct{}
	writerAccepted       map[pipelineDestinationOccurrence]struct{}
	writerFamilyRejects  map[string]promcollector.PipelineReason
	writerSeriesRejects  map[pipelineDestinationOccurrence]promcollector.PipelineReason
	writerRejectFacts    map[pipelineDestinationOccurrence]promcollector.PipelineDiagnostic
	selectorRejected     map[prompkg.RawSampleIdentity]struct{}
	rawAccepted          map[prompkg.RawSampleIdentity]struct{}
	originsByRaw         map[prompkg.RawSampleIdentity]pipelineDestinationOccurrence
	originsBySource      map[prompkg.SampleSeriesIdentity]map[pipelineDestinationOccurrence]struct{}
	relabelDropped       map[pipelinePhysicalOccurrence]map[pipelineRelabelLocation]promcollector.PipelineDiagnostic
	blockEntries         map[pipelinePhysicalOccurrence]map[pipelineRelabelLocation]string
	rulesEvaluated       map[pipelinePhysicalOccurrence]map[pipelineRuleKey]promcollector.PipelineDiagnostic
	destinations         map[pipelineDestinationOccurrence]map[pipelineDestinationOccurrence]struct{}
	stageOrder           map[pipelineStageKey]int
	initialNodes         map[prompkg.RawSampleIdentity]pipelinePhysicalOccurrence
	transitionsByInput   map[pipelinePhysicalOccurrence][]orderedPipelineTransition
	lineageNodesByOrigin map[prompkg.RawSampleIdentity][]pipelinePhysicalOccurrence
	finalRawByOrigin     map[prompkg.RawSampleIdentity]prompkg.RawSampleIdentity
	lastOutputByOrigin   map[prompkg.RawSampleIdentity]promcollector.PipelineDiagnostic
	typedRejects         map[prompkg.SampleSeriesIdentity]struct{}
	relabelLocations     map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{}
	selectedProfiles     map[string]struct{}
	selectedProfileOrder []string
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
			discards:           make(map[relabelDiscardRuleKey]*relabelDiscardAudit),
			nameRewrites:       make(map[relabelDiscardRuleKey]*relabelNameRewriteAudit),
			blockInputs:        make(map[pipelineRelabelLocation]map[string]struct{}),
			provenance:         make(pipelineProvenance),
			qualifyProfilePath: len(profiles) > 1,
			identityCollapse: relabelIdentityCollapseAudit{
				locations: make(map[pipelineRuleKey]struct{}),
			},
			typedFamilyRejects: make(map[string]relabelTypedFamilyRejectAudit),
			invalidNameDrops: relabelInvalidNameDropAudit{
				blocks:            make(map[pipelineRelabelLocation]struct{}),
				families:          make(map[string]struct{}),
				logicalIdentities: make(map[prompkg.SampleSeriesIdentity]struct{}),
				rawSamples:        make(map[prompkg.RawSampleIdentity]struct{}),
			},
		},
		blockInputLabels:     make(map[pipelineBlockSampleKey]map[string]struct{}),
		sourcesByFamily:      make(map[string]map[prompkg.SampleSeriesIdentity]struct{}),
		writerAccepted:       make(map[pipelineDestinationOccurrence]struct{}),
		writerFamilyRejects:  make(map[string]promcollector.PipelineReason),
		writerSeriesRejects:  make(map[pipelineDestinationOccurrence]promcollector.PipelineReason),
		writerRejectFacts:    make(map[pipelineDestinationOccurrence]promcollector.PipelineDiagnostic),
		selectorRejected:     make(map[prompkg.RawSampleIdentity]struct{}),
		rawAccepted:          make(map[prompkg.RawSampleIdentity]struct{}),
		originsByRaw:         make(map[prompkg.RawSampleIdentity]pipelineDestinationOccurrence),
		originsBySource:      make(map[prompkg.SampleSeriesIdentity]map[pipelineDestinationOccurrence]struct{}),
		relabelDropped:       make(map[pipelinePhysicalOccurrence]map[pipelineRelabelLocation]promcollector.PipelineDiagnostic),
		blockEntries:         make(map[pipelinePhysicalOccurrence]map[pipelineRelabelLocation]string),
		rulesEvaluated:       make(map[pipelinePhysicalOccurrence]map[pipelineRuleKey]promcollector.PipelineDiagnostic),
		destinations:         make(map[pipelineDestinationOccurrence]map[pipelineDestinationOccurrence]struct{}),
		initialNodes:         make(map[prompkg.RawSampleIdentity]pipelinePhysicalOccurrence),
		transitionsByInput:   make(map[pipelinePhysicalOccurrence][]orderedPipelineTransition),
		stageOrder:           make(map[pipelineStageKey]int),
		lineageNodesByOrigin: make(map[prompkg.RawSampleIdentity][]pipelinePhysicalOccurrence),
		finalRawByOrigin:     make(map[prompkg.RawSampleIdentity]prompkg.RawSampleIdentity),
		lastOutputByOrigin:   make(map[prompkg.RawSampleIdentity]promcollector.PipelineDiagnostic),
		typedRejects:         make(map[prompkg.SampleSeriesIdentity]struct{}),
		selectedProfiles:     make(map[string]struct{}),
	}
	for index, stage := range stages {
		summary.stageOrder[pipelineStageKey{stage: stage.stage, profile: stage.profile}] = index
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
	return pipelineDestinationOccurrence{
		series: fact.Source,
		value:  fact.ValueIdentity,
		scalar: fact.ScalarValue,
	}
}

func diagnosticLocation(fact promcollector.PipelineDiagnostic) pipelineRelabelLocation {
	return pipelineRelabelLocation{stage: fact.RelabelStage, profile: fact.ProfileName, block: fact.BlockIndex}
}

func diagnosticNode(fact promcollector.PipelineDiagnostic) pipelinePhysicalOccurrence {
	return pipelinePhysicalOccurrence{raw: fact.RawIdentity, occurrence: diagnosticOccurrence(fact)}
}

func diagnosticStage(fact promcollector.PipelineDiagnostic) pipelineStageKey {
	return pipelineStageKey{stage: fact.RelabelStage, profile: fact.ProfileName}
}

func (s *pipelineDiagnosticSummary) addTransition(
	input pipelinePhysicalOccurrence,
	stage pipelineStageKey,
	transition pipelineTransition,
) {
	order, ok := s.stageOrder[stage]
	if !ok {
		return
	}
	s.transitionsByInput[input] = append(s.transitionsByInput[input], orderedPipelineTransition{
		order:      order,
		transition: transition,
	})
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
		s.initialNodes[fact.RawIdentity] = pipelinePhysicalOccurrence{raw: fact.RawIdentity, occurrence: occurrence}
		s.originsByRaw[fact.RawIdentity] = occurrence
		origins := s.originsBySource[fact.Source]
		if origins == nil {
			origins = make(map[pipelineDestinationOccurrence]struct{})
			s.originsBySource[fact.Source] = origins
		}
		origins[occurrence] = struct{}{}
	case promcollector.PipelineRelabelBlockEntered:
		location := diagnosticLocation(fact)
		node := diagnosticNode(fact)
		entries := s.blockEntries[node]
		if entries == nil {
			entries = make(map[pipelineRelabelLocation]string)
			s.blockEntries[node] = entries
		}
		entries[location] = fact.InputMetricName
		inputs := s.audits.blockInputs[location]
		if inputs == nil {
			inputs = make(map[string]struct{})
			s.audits.blockInputs[location] = inputs
		}
		inputs[fact.InputMetricName] = struct{}{}
		key := pipelineBlockSampleKey{raw: fact.RawIdentity, location: location}
		labels := s.blockInputLabels[key]
		if labels == nil {
			labels = make(map[string]struct{})
			s.blockInputLabels[key] = labels
		}
		for _, name := range fact.InputLabelNames {
			labels[name] = struct{}{}
		}
	case promcollector.PipelineRelabelRuleEvaluated:
		location := diagnosticLocation(fact)
		node := diagnosticNode(fact)
		rules := s.rulesEvaluated[node]
		if rules == nil {
			rules = make(map[pipelineRuleKey]promcollector.PipelineDiagnostic)
			s.rulesEvaluated[node] = rules
		}
		rules[pipelineRuleKey{location: location, rule: fact.RuleIndex}] = fact
		key := relabelDiscardRuleKey{
			stage: fact.RelabelStage, profile: fact.ProfileName, block: fact.BlockIndex, rule: fact.RuleIndex,
		}
		if audit := s.audits.nameRewrites[key]; audit != nil && fact.RelabelRuleMatched {
			audit.metricNames[fact.InputMetricName] = struct{}{}
			for label := range s.blockInputLabels[pipelineBlockSampleKey{raw: fact.RawIdentity, location: location}] {
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
		location := diagnosticLocation(fact)
		node := diagnosticNode(fact)
		drops := s.relabelDropped[node]
		if drops == nil {
			drops = make(map[pipelineRelabelLocation]promcollector.PipelineDiagnostic)
			s.relabelDropped[node] = drops
		}
		drops[location] = fact
		s.addTransition(node, diagnosticStage(fact), pipelineTransition{
			fact: fact, dropped: true,
		})
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
		input := pipelinePhysicalOccurrence{raw: fact.RawIdentity, occurrence: source}
		output := pipelinePhysicalOccurrence{raw: fact.DestinationRawIdentity, occurrence: destination}
		s.addTransition(input, diagnosticStage(fact), pipelineTransition{
			fact: fact, output: output,
		})
	case promcollector.PipelineTypedFamilyRejected:
		if fact.Destination.Family != "" {
			s.typedRejects[fact.Destination] = struct{}{}
		}
	case promcollector.PipelineWriterFamilyRejected:
		s.writerFamilyRejects[fact.MetricName] = fact.Reason
	case promcollector.PipelineWriterSeriesRejected:
		destination := pipelineDestinationOccurrence{
			series: fact.Destination,
			value:  fact.ValueIdentity,
			scalar: fact.ScalarValue,
		}
		s.writerSeriesRejects[destination] = fact.Reason
		s.writerRejectFacts[destination] = fact
	case promcollector.PipelineWriterSeriesAccepted:
		destination := pipelineDestinationOccurrence{
			series: fact.Destination,
			value:  fact.ValueIdentity,
			scalar: fact.ScalarValue,
		}
		s.writerAccepted[destination] = struct{}{}
	case promcollector.PipelineProfileSelected:
		if _, seen := s.selectedProfiles[fact.ProfileName]; !seen {
			s.selectedProfileOrder = append(s.selectedProfileOrder, fact.ProfileName)
		}
		s.selectedProfiles[fact.ProfileName] = struct{}{}
	}
}

func (s *pipelineDiagnosticSummary) finalRawForOrigin(
	raw prompkg.RawSampleIdentity,
) (prompkg.RawSampleIdentity, promcollector.PipelineDiagnostic, bool) {
	physical, ok := s.finalRawByOrigin[raw]
	if !ok {
		return raw, promcollector.PipelineDiagnostic{}, false
	}
	fact, hasOutput := s.lastOutputByOrigin[raw]
	return physical, fact, hasOutput
}

func (s *pipelineDiagnosticSummary) lineageNodes(raw prompkg.RawSampleIdentity) []pipelinePhysicalOccurrence {
	if nodes := s.lineageNodesByOrigin[raw]; len(nodes) > 0 {
		return nodes
	}
	if node, ok := s.initialNodes[raw]; ok {
		return []pipelinePhysicalOccurrence{node}
	}
	return nil
}

func (s *pipelineDiagnosticSummary) rebuildLineages(ctx context.Context) error {
	clear(s.lineageNodesByOrigin)
	clear(s.finalRawByOrigin)
	clear(s.lastOutputByOrigin)
	for input := range s.transitionsByInput {
		if err := ctx.Err(); err != nil {
			return err
		}
		slices.SortFunc(s.transitionsByInput[input], func(a, b orderedPipelineTransition) int {
			return a.order - b.order
		})
	}
	for raw, initial := range s.initialNodes {
		if err := ctx.Err(); err != nil {
			return err
		}
		current := initial
		nodes := []pipelinePhysicalOccurrence{current}
		s.finalRawByOrigin[raw] = current.raw
		nextOrder := 0
		for {
			if err := ctx.Err(); err != nil {
				return err
			}
			var next orderedPipelineTransition
			found := false
			candidates := s.transitionsByInput[current]
			for index := range candidates {
				if candidates[index].order >= nextOrder {
					next = candidates[index]
					found = true
					break
				}
			}
			if !found {
				break
			}
			nextOrder = next.order + 1
			if next.transition.dropped {
				break
			}
			current = next.transition.output
			s.finalRawByOrigin[raw] = current.raw
			s.lastOutputByOrigin[raw] = next.transition.fact
			if current != nodes[len(nodes)-1] {
				nodes = append(nodes, current)
			}
		}
		s.lineageNodesByOrigin[raw] = nodes
	}
	return nil
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
	for _, node := range s.lineageNodes(raw) {
		for _, fact := range s.relabelDropped[node] {
			return fact, true
		}
	}
	return promcollector.PipelineDiagnostic{}, false
}

func (s *pipelineDiagnosticSummary) blockEntryForRaw(
	raw prompkg.RawSampleIdentity,
	location pipelineRelabelLocation,
) (string, bool) {
	for _, node := range s.lineageNodes(raw) {
		if entry, ok := s.blockEntries[node][location]; ok {
			return entry, true
		}
	}
	return "", false
}

func (s *pipelineDiagnosticSummary) ruleForRaw(
	raw prompkg.RawSampleIdentity,
	key pipelineRuleKey,
) (promcollector.PipelineDiagnostic, bool) {
	for _, node := range s.lineageNodes(raw) {
		if fact, ok := s.rulesEvaluated[node][key]; ok {
			return fact, true
		}
	}
	return promcollector.PipelineDiagnostic{}, false
}

func (s *pipelineDiagnosticSummary) rulesForRaw(
	raw prompkg.RawSampleIdentity,
) map[pipelineRuleKey]promcollector.PipelineDiagnostic {
	rules := make(map[pipelineRuleKey]promcollector.PipelineDiagnostic)
	for _, node := range s.lineageNodes(raw) {
		maps.Copy(rules, s.rulesEvaluated[node])
	}
	return rules
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

func (s *pipelineDiagnosticSummary) relabelMutationIndexes(
	ctx context.Context,
) (
	map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{},
	map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{},
	error,
) {
	mutations := make(map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{})
	mutationsAndDrops := make(map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{})
	for raw, occurrence := range s.originsByRaw {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		for key, fact := range s.rulesForRaw(raw) {
			if fact.RelabelRuleDropped {
				addPipelineRuleLocation(mutationsAndDrops, occurrence.series, key)
				continue
			}
			if !fact.RelabelRuleMatched {
				continue
			}
			if fact.InputMetricName != fact.OutputMetricName || !slices.Equal(fact.InputLabels, fact.OutputLabels) {
				addPipelineRuleLocation(mutations, occurrence.series, key)
				addPipelineRuleLocation(mutationsAndDrops, occurrence.series, key)
			}
		}
	}
	return mutations, mutationsAndDrops, nil
}

func addPipelineRuleLocation(
	index map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{},
	source prompkg.SampleSeriesIdentity,
	key pipelineRuleKey,
) {
	locations := index[source]
	if locations == nil {
		locations = make(map[pipelineRuleKey]struct{})
		index[source] = locations
	}
	locations[key] = struct{}{}
}

func unionPipelineRuleLocations(
	index map[prompkg.SampleSeriesIdentity]map[pipelineRuleKey]struct{},
	sources map[prompkg.SampleSeriesIdentity]struct{},
) map[pipelineRuleKey]struct{} {
	locations := make(map[pipelineRuleKey]struct{})
	for source := range sources {
		for key := range index[source] {
			locations[key] = struct{}{}
		}
	}
	return locations
}

func (s *pipelineDiagnosticSummary) relabelPolicyPaths(
	sources map[prompkg.SampleSeriesIdentity]struct{},
) []string {
	if s == nil || len(sources) == 0 {
		return nil
	}
	locations := unionPipelineRuleLocations(s.relabelLocations, sources)
	return sortedRelabelRuleLocationPaths(locations, s.audits.qualifyProfilePath)
}

func (s *pipelineDiagnosticSummary) finalize(ctx context.Context) (relabelPolicyAudits, error) {
	if s == nil {
		return relabelPolicyAudits{}, nil
	}
	if err := ctx.Err(); err != nil {
		return relabelPolicyAudits{}, err
	}
	if err := s.rebuildLineages(ctx); err != nil {
		return relabelPolicyAudits{}, err
	}
	for _, audit := range s.audits.discards {
		audit.rawSeries = len(audit.rawSamples)
	}
	s.audits.invalidNameDrops.rawSeries = len(s.audits.invalidNameDrops.rawSamples)
	for source, origins := range s.originsBySource {
		if err := ctx.Err(); err != nil {
			return relabelPolicyAudits{}, err
		}
		for origin := range origins {
			queue := []pipelineDestinationOccurrence{origin}
			seen := make(map[pipelineDestinationOccurrence]struct{})
			for len(queue) > 0 {
				if err := ctx.Err(); err != nil {
					return relabelPolicyAudits{}, err
				}
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
		if err := ctx.Err(); err != nil {
			return relabelPolicyAudits{}, err
		}
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
		if err := ctx.Err(); err != nil {
			return relabelPolicyAudits{}, err
		}
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
	mutations, mutationsAndDrops, err := s.relabelMutationIndexes(ctx)
	if err != nil {
		return relabelPolicyAudits{}, err
	}
	s.relabelLocations = mutationsAndDrops
	s.audits.identityCollapse.locations = unionPipelineRuleLocations(mutations, collapsedSources)

	sourcesByRejectedIdentity := make(map[prompkg.SampleSeriesIdentity]map[prompkg.SampleSeriesIdentity]struct{})
	for origin, fact := range s.lastOutputByOrigin {
		if err := ctx.Err(); err != nil {
			return relabelPolicyAudits{}, err
		}
		if _, rejected := s.typedRejects[fact.Destination]; !rejected {
			continue
		}
		source, ok := s.originsByRaw[origin]
		if !ok {
			continue
		}
		sources := sourcesByRejectedIdentity[fact.Destination]
		if sources == nil {
			sources = make(map[prompkg.SampleSeriesIdentity]struct{})
			sourcesByRejectedIdentity[fact.Destination] = sources
		}
		sources[source.series] = struct{}{}
	}
	for identity := range s.typedRejects {
		if err := ctx.Err(); err != nil {
			return relabelPolicyAudits{}, err
		}
		audit := s.audits.typedFamilyRejects[identity.Family]
		if audit.locations == nil {
			audit.locations = make(map[pipelineRuleKey]struct{})
		}
		for key := range unionPipelineRuleLocations(mutationsAndDrops, sourcesByRejectedIdentity[identity]) {
			audit.locations[key] = struct{}{}
		}
		s.audits.typedFamilyRejects[identity.Family] = audit
	}
	return s.audits, nil
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
		path := relabelRuleLocationPath(
			pipelineRuleKey{
				location: pipelineRelabelLocation{stage: key.stage, profile: key.profile, block: key.block},
				rule:     key.rule,
			},
			audits.qualifyProfilePath,
		)
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

	for _, family := range slices.Sorted(maps.Keys(audits.typedFamilyRejects)) {
		audit := audits.typedFamilyRejects[family]
		locations := sortedRelabelRuleLocationPaths(audit.locations, audits.qualifyProfilePath)
		r.addError(
			"typed_family_relabel_rejected",
			aggregateRelabelPath("relabeling", locations, audits.qualifyProfilePath),
			fmt.Sprintf("typed family %q was rejected after mutations at %v", family, locations),
			"Relabeling must preserve every component of a histogram or summary as one valid typed family. Inspect each listed owner and rule that changed or dropped a contributing component.",
		)
	}
}

func aggregateRelabelPath(fallback string, locations []string, qualifyProfile bool) string {
	if qualifyProfile && len(locations) > 0 {
		return strings.Join(locations, ", ")
	}
	return fallback
}

func relabelBlockLocationPath(location pipelineRelabelLocation, qualifyProfile bool) string {
	if location.stage == promcollector.PipelineRelabelStageProfile && qualifyProfile {
		return fmt.Sprintf("profiles[%s].relabeling[%d]", location.profile, location.block)
	}
	return relabelBlockPath(location.stage, location.block)
}

func relabelRuleLocationPath(key pipelineRuleKey, qualifyProfile bool) string {
	return fmt.Sprintf(
		"%s.metric_relabel_configs[%d]",
		relabelBlockLocationPath(key.location, qualifyProfile),
		key.rule,
	)
}

func sortedRelabelRuleLocationPaths(locations map[pipelineRuleKey]struct{}, qualifyProfile bool) []string {
	paths := make([]string, 0, len(locations))
	for location := range locations {
		paths = append(paths, relabelRuleLocationPath(location, qualifyProfile))
	}
	slices.Sort(paths)
	return paths
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
