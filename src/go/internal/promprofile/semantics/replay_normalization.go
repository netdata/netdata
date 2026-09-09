// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

const semanticMetricNameField = "__name__"

type normalizationReplayState struct {
	metric string
	labels map[string]string
}

type expectedNormalizationReplay struct {
	node     *compiledNormalization
	branch   string
	before   normalizationReplayState
	after    normalizationReplayState
	writes   map[string]struct{}
	terminal bool
}

type observedNormalizationMutations struct {
	paths map[string][]string
	first map[string]int
	last  map[string]int
}

// ReconcileProductionNormalizations executes the compiled semantic
// transformation graph and assigns every observed production name/label
// mutation to exactly one active normalization. Production rule paths are
// retained only as replay evidence; authored contracts do not reference them.
func (c *CompiledSemanticCase) ReconcileProductionNormalizations(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production normalization reconciliation"); err != nil {
		return err
	}
	if c == nil || c.root == nil {
		return fmt.Errorf("production normalization reconciliation: compiled semantic case is nil")
	}
	if snapshot == nil || reconciled == nil {
		return fmt.Errorf("production normalization reconciliation: snapshot and reconciled sources must be present")
	}
	if len(reconciled.Sources) != len(snapshot.Sources) {
		return fmt.Errorf("production normalization reconciliation: source join has %d entries for %d production sources",
			len(reconciled.Sources), len(snapshot.Sources))
	}

	facts := make([]ReconciledSemanticNormalization, 0)
	var errs []error
	for _, binding := range reconciled.Sources {
		if err := checkSemanticContext(ctx, "during production normalization reconciliation"); err != nil {
			return err
		}
		if binding.SourceIndex < 0 || binding.SourceIndex >= len(snapshot.Sources) ||
			binding.program == nil || binding.occurrence == nil {
			errs = append(errs, fmt.Errorf("source join contains an invalid binding at index %d", binding.SourceIndex))
			continue
		}
		sourceFacts, err := reconcileProductionSourceNormalizations(binding, snapshot.Sources[binding.SourceIndex])
		if err != nil {
			errs = append(errs, fmt.Errorf("raw occurrence %s (%s/%s): %w",
				snapshot.Sources[binding.SourceIndex].OccurrenceID,
				snapshot.Sources[binding.SourceIndex].MetricName,
				snapshot.Sources[binding.SourceIndex].Component,
				err,
			))
			continue
		}
		facts = append(facts, sourceFacts...)
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	reconciled.Normalizations = facts
	return nil
}

func reconcileProductionSourceNormalizations(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
) ([]ReconciledSemanticNormalization, error) {
	expected, want, err := expectedProductionNormalizations(binding, source)
	if err != nil {
		return nil, err
	}
	observed, got, err := observeProductionNormalizations(binding, source, expected)
	if err != nil {
		return nil, err
	}
	if !sameNormalizationReplayState(got, want) {
		return nil, fmt.Errorf("post-relabel state got %s, want %s",
			formatNormalizationReplayState(got), formatNormalizationReplayState(want))
	}

	facts := make([]ReconciledSemanticNormalization, 0, len(expected))
	active := make(map[string]expectedNormalizationReplay, len(expected))
	for _, step := range expected {
		active[step.node.id] = step
		paths := observed.paths[step.node.id]
		changed := step.terminal || !sameNormalizationReplayState(step.before, step.after)
		switch {
		case changed && len(paths) == 0:
			return nil, fmt.Errorf("normalization %q branch %q has no production mutation", step.node.id, step.branch)
		case !changed && len(paths) != 0:
			return nil, fmt.Errorf("normalization %q branch %q unexpectedly owns production mutations %v",
				step.node.id, step.branch, paths)
		}
		facts = append(facts, ReconciledSemanticNormalization{
			SourceIndex:   binding.SourceIndex,
			Profile:       binding.Profile,
			Normalization: step.node.id,
			Branch:        step.branch,
			RuntimePaths:  slices.Clone(paths),
			Terminal:      step.terminal,
		})
	}
	for id, first := range observed.first {
		step := active[id]
		for predecessor := range step.node.predecessors {
			if _, ok := active[predecessor]; !ok {
				continue
			}
			if last, ok := observed.last[predecessor]; ok && last >= first {
				return nil, fmt.Errorf("normalization %q mutates before dependency %q completes", id, predecessor)
			}
		}
	}
	return facts, nil
}

func expectedProductionNormalizations(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
) ([]expectedNormalizationReplay, normalizationReplayState, error) {
	state := normalizationReplayState{
		metric: source.MetricName,
		labels: semanticPipelineLabelMap(source.Labels),
	}
	var expected []expectedNormalizationReplay
	for _, node := range binding.program.normalizations {
		if !slices.Contains(node.occurrences, binding.occurrence.key) {
			continue
		}
		if node.definition.AppliesTo != nil &&
			!replayLabelConditionMatches(node.definition.AppliesTo.Where, state.labels) {
			continue
		}
		step, err := applyExpectedNormalization(binding, node, source.Component, state)
		if err != nil {
			return nil, normalizationReplayState{}, fmt.Errorf("normalization %q: %w", node.id, err)
		}
		expected = append(expected, step)
		state = step.after
		if step.terminal {
			break
		}
	}
	return expected, state, nil
}

func applyExpectedNormalization(
	binding ReconciledSemanticSource,
	node *compiledNormalization,
	component string,
	input normalizationReplayState,
) (expectedNormalizationReplay, error) {
	definition := node.definition
	step := expectedNormalizationReplay{
		node:   node,
		before: cloneNormalizationReplayState(input),
		after:  cloneNormalizationReplayState(input),
		writes: normalizationReplayWrites(definition),
	}
	switch definition.Kind {
	case "category":
		value, present := step.after.labels[definition.SourceLabel]
		if definition.SourceLabel == semanticMetricNameField {
			value, present = step.after.metric, true
		}
		target, branch := replayCategoryValue(definition, value, present)
		step.branch = branch
		if target != nil {
			step.after.labels[definition.TargetLabel] = *target
		}
	case "label_rename":
		value, present := step.after.labels[definition.SourceLabel]
		if !present {
			step.branch = "absent"
			break
		}
		step.branch = "present"
		if _, collision := step.after.labels[definition.TargetLabel]; collision {
			return step, fmt.Errorf("target label %q already exists", definition.TargetLabel)
		}
		step.after.labels[definition.TargetLabel] = value
		if !*definition.RetainSource {
			delete(step.after.labels, definition.SourceLabel)
		}
	case "finite_alias", "namespace_alias":
		family, ok := replaySourceFamily(step.after.metric, component)
		if !ok {
			return step, fmt.Errorf("cannot derive source family from %q/%q", step.after.metric, component)
		}
		target, ok := node.familyAliases[family]
		if !ok {
			return step, fmt.Errorf("source family %q has no compiled alias mapping", family)
		}
		step.branch = "family:" + family
		step.after.metric = replayMetricForFamily(target, component)
	case "embedded_identity_repair", "embedded_identity_extract":
		branch, capture, canonical, err := replayEmbeddedNormalizationBranch(binding, step.after.metric, component)
		if err != nil {
			return step, err
		}
		if branch == "canonical" {
			step.branch = branch
			break
		}
		if definition.Kind == "embedded_identity_extract" {
			step.branch = "embedded"
			if _, collision := step.after.labels[definition.TargetLabel]; collision {
				return step, fmt.Errorf("target label %q already exists", definition.TargetLabel)
			}
			step.after.metric = replayMetricForFamily(canonical, component)
			step.after.labels[definition.TargetLabel] = capture
			break
		}
		sourceIdentity, present := step.after.labels[definition.SourceIdentityLabel]
		if !present {
			step.branch = "embedded:identity_absent"
			step.terminal = true
			break
		}
		step.branch = "embedded:identity_present"
		step.after.metric = replayMetricForFamily(canonical, component)
		step.after.labels[definition.Canonical.IdentityLabel] = joinNonblankIdentity(
			definition.Identity.Separator,
			map[string]string{
				definition.Embedded.Capture:    capture,
				definition.SourceIdentityLabel: sourceIdentity,
			},
			definition.Identity.Operands,
		)
	case "generated_component_exclusion":
		family, ok := replaySourceFamily(step.after.metric, component)
		if !ok || !strings.HasPrefix(family, definition.Source.NamespacePrefix) ||
			!strings.HasSuffix(family, definition.Source.TerminalSuffix) {
			return step, fmt.Errorf("generated exclusion does not match source family %q", family)
		}
		step.branch = "generated_member"
		step.terminal = true
	default:
		panic("validated normalization kind has no replay implementation")
	}
	return step, nil
}

func observeProductionNormalizations(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
	expected []expectedNormalizationReplay,
) (observedNormalizationMutations, normalizationReplayState, error) {
	result := observedNormalizationMutations{
		paths: make(map[string][]string),
		first: make(map[string]int),
		last:  make(map[string]int),
	}
	state := normalizationReplayState{metric: source.MetricName, labels: semanticPipelineLabelMap(source.Labels)}
	owners := make(map[string][]string)
	terminalOwner := ""
	for _, step := range expected {
		for field := range step.writes {
			owners[field] = append(owners[field], step.node.id)
		}
		if step.terminal {
			terminalOwner = step.node.id
		}
	}
	lastMutationOwner := ""
	closedOwners := make(map[string]struct{})
	mutationIndex := 0
	terminalPath := ""
	for _, occurrence := range source.RelabelRules {
		input := normalizationReplayState{
			metric: occurrence.InputMetricName,
			labels: semanticPipelineLabelMap(occurrence.InputLabels),
		}
		if !sameNormalizationReplayState(input, state) {
			return result, state, fmt.Errorf("production relabel trace path %q starts from %s, want %s",
				occurrence.RuntimePath, formatNormalizationReplayState(input), formatNormalizationReplayState(state))
		}
		output := normalizationReplayState{
			metric: occurrence.OutputMetricName,
			labels: semanticPipelineLabelMap(occurrence.OutputLabels),
		}
		if !occurrence.Matched && !sameNormalizationReplayState(input, output) {
			return result, state, fmt.Errorf("unmatched production relabel path %q changes state", occurrence.RuntimePath)
		}
		changedFields := normalizationReplayChangedFields(input, output)
		if (len(changedFields) != 0 || occurrence.Dropped) && occurrence.Profile != binding.Profile {
			return result, state, fmt.Errorf("production relabel path %q mutates source owned by profile %q from profile %q",
				occurrence.RuntimePath, binding.Profile, occurrence.Profile)
		}
		owner := ""
		for _, field := range changedFields {
			candidates := owners[field]
			if len(candidates) == 0 {
				return result, state, fmt.Errorf("production relabel path %q mutates unowned field %q",
					occurrence.RuntimePath, field)
			}
			if len(candidates) != 1 {
				return result, state, fmt.Errorf("production relabel path %q mutates field %q owned by normalizations %v",
					occurrence.RuntimePath, field, candidates)
			}
			if owner != "" && owner != candidates[0] {
				return result, state, fmt.Errorf("production relabel path %q combines normalization purposes %q and %q",
					occurrence.RuntimePath, owner, candidates[0])
			}
			owner = candidates[0]
		}
		if occurrence.Dropped && terminalOwner != "" {
			if !occurrence.Matched || occurrence.Action != "drop" {
				return result, state, fmt.Errorf("production relabel path %q implements normalization terminal with action=%q matched=%t",
					occurrence.RuntimePath, occurrence.Action, occurrence.Matched)
			}
			if owner != "" && owner != terminalOwner {
				return result, state, fmt.Errorf("production relabel path %q combines normalization mutation %q with terminal %q",
					occurrence.RuntimePath, owner, terminalOwner)
			}
			if terminalPath != "" {
				return result, state, fmt.Errorf("normalization %q has multiple production terminal paths", terminalOwner)
			}
			terminalPath = occurrence.RuntimePath
			owner = terminalOwner
		}
		state = output
		if owner == "" {
			continue
		}
		if _, closed := closedOwners[owner]; closed {
			return result, state, fmt.Errorf("normalization %q has a non-contiguous production rule chain", owner)
		}
		if lastMutationOwner != "" && lastMutationOwner != owner {
			closedOwners[lastMutationOwner] = struct{}{}
		}
		lastMutationOwner = owner
		if _, ok := result.first[owner]; !ok {
			result.first[owner] = mutationIndex
		}
		result.last[owner] = mutationIndex
		result.paths[owner] = append(result.paths[owner], occurrence.RuntimePath)
		mutationIndex++
	}

	if terminalOwner != "" {
		if source.Terminal == nil || source.Terminal.Disposition != "profile_excluded" ||
			source.Terminal.Profile != binding.Profile {
			return result, state, fmt.Errorf("normalization %q expects profile exclusion, got %#v",
				terminalOwner, source.Terminal)
		}
		if source.Terminal.RuntimePath != terminalPath {
			return result, state, fmt.Errorf("normalization %q terminal path got %q, want %q",
				terminalOwner, source.Terminal.RuntimePath, terminalPath)
		}
	}
	if terminalOwner == "" && (source.Terminal == nil || source.Terminal.Disposition != "profile_excluded") {
		final := normalizationReplayState{
			metric: source.FinalMetricName,
			labels: semanticPipelineLabelMap(source.FinalLabels),
		}
		if !sameNormalizationReplayState(final, state) {
			return result, state, fmt.Errorf("production final state %s differs from relabel trace %s",
				formatNormalizationReplayState(final), formatNormalizationReplayState(state))
		}
	}
	return result, state, nil
}

func replayEmbeddedNormalizationBranch(
	binding ReconciledSemanticSource,
	metric string,
	component string,
) (branch, capture, canonical string, err error) {
	if binding.entry.canonical == nil || binding.entry.embedded == nil {
		return "", "", "", fmt.Errorf("embedded normalization has no compiled grammar form %q", binding.entry.formID)
	}
	family, ok := replaySourceFamily(metric, component)
	if !ok {
		return "", "", "", fmt.Errorf("cannot derive source family from %q/%q", metric, component)
	}
	canonical = binding.entry.canonical.Prefix + binding.entry.canonical.Suffix
	if family == canonical {
		return "canonical", "", canonical, nil
	}
	tail := binding.entry.embedded.Separator + binding.entry.embedded.Suffix
	if !embeddedFamilyNamespaceMatches(family, *binding.entry.embedded) || !strings.HasSuffix(family, tail) {
		return "", "", "", fmt.Errorf("family %q is outside grammar form %q", family, binding.entry.formID)
	}
	capture = strings.TrimSuffix(strings.TrimPrefix(family, binding.entry.embedded.Prefix), tail)
	if capture == "" {
		return "", "", "", fmt.Errorf("family %q has an empty embedded identity", family)
	}
	return "embedded", capture, canonical, nil
}

func normalizationReplayWrites(definition Normalization) map[string]struct{} {
	writes := make(map[string]struct{})
	switch definition.Kind {
	case "category":
		writes[definition.TargetLabel] = struct{}{}
	case "label_rename":
		writes[definition.TargetLabel] = struct{}{}
		if !*definition.RetainSource {
			writes[definition.SourceLabel] = struct{}{}
		}
	case "finite_alias", "namespace_alias":
		writes[semanticMetricNameField] = struct{}{}
	case "embedded_identity_repair":
		writes[semanticMetricNameField] = struct{}{}
		writes[definition.Canonical.IdentityLabel] = struct{}{}
		// The bounded grammar capture may be materialized as a scratch label
		// while production composes the canonical identity. Exact final-state
		// reconciliation still requires the scratch label to be removed.
		writes[definition.Embedded.Capture] = struct{}{}
	case "embedded_identity_extract":
		writes[semanticMetricNameField] = struct{}{}
		writes[definition.TargetLabel] = struct{}{}
	case "generated_component_exclusion":
		// Terminal ownership is reconciled separately from name and label writes.
	default:
		panic("validated normalization kind has no replay writes")
	}
	return writes
}

func replayCategoryValue(definition Normalization, value string, present bool) (*string, string) {
	if !present {
		return definition.Missing.Set, "missing"
	}
	if target, ok := definition.Exact[value]; ok {
		return &target, "exact:" + value
	}
	parsed, ok := parseCanonicalUint64(value)
	if !ok {
		return definition.Malformed.Set, "malformed"
	}
	for _, valueRange := range definition.Ranges {
		if parsed >= *valueRange.Min && parsed <= *valueRange.Max {
			return &valueRange.Value, fmt.Sprintf("range:%d-%d", *valueRange.Min, *valueRange.Max)
		}
	}
	return definition.Unknown.Set, "unknown"
}

func replayMetricForFamily(family, component string) string {
	switch component {
	case "histogram_bucket":
		return family + "_bucket"
	case "histogram_sum", "summary_sum":
		return family + "_sum"
	case "histogram_count", "summary_count":
		return family + "_count"
	default:
		return family
	}
}

func replayLabelConditionMatches(condition *LabelCondition, labels map[string]string) bool {
	if condition == nil {
		return true
	}
	for _, clause := range condition.Any {
		matched := true
		for _, predicate := range clause.All {
			value, present := labels[predicate.Label]
			switch predicate.Op {
			case "absent":
				matched = !present
			case "present", "nonblank":
				matched = present
			case "eq":
				matched = present && value == *predicate.Value
			case "in":
				matched = present && slices.Contains(predicate.Values, value)
			default:
				panic("validated label predicate has no replay matcher")
			}
			if !matched {
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func joinNonblankIdentity(separator string, values map[string]string, operands []string) string {
	joined := make([]string, 0, len(operands))
	for _, operand := range operands {
		if value := values[operand]; strings.TrimSpace(value) != "" {
			joined = append(joined, value)
		}
	}
	return strings.Join(joined, separator)
}

func normalizationReplayChangedFields(left, right normalizationReplayState) []string {
	var fields []string
	if left.metric != right.metric {
		fields = append(fields, semanticMetricNameField)
	}
	for name, value := range left.labels {
		if next, ok := right.labels[name]; !ok || next != value {
			fields = append(fields, name)
		}
	}
	for name := range right.labels {
		if _, ok := left.labels[name]; !ok {
			fields = append(fields, name)
		}
	}
	slices.Sort(fields)
	return fields
}

func semanticPipelineLabelMap(values []promreplay.SemanticLabel) map[string]string {
	out := make(map[string]string, len(values))
	for _, label := range values {
		if strings.TrimSpace(label.Value) != "" {
			out[label.Name] = label.Value
		}
	}
	return out
}

func cloneNormalizationReplayState(state normalizationReplayState) normalizationReplayState {
	return normalizationReplayState{metric: state.metric, labels: maps.Clone(state.labels)}
}

func sameNormalizationReplayState(left, right normalizationReplayState) bool {
	return left.metric == right.metric && maps.Equal(left.labels, right.labels)
}

func formatNormalizationReplayState(state normalizationReplayState) string {
	pairs := make([]string, 0, len(state.labels))
	for _, name := range slices.Sorted(maps.Keys(state.labels)) {
		pairs = append(pairs, name+"="+state.labels[name])
	}
	return fmt.Sprintf("name=%q labels={%s}", state.metric, strings.Join(pairs, ","))
}
