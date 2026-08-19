// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

// ReconcileProductionClaims validates source relationship and state-encoding
// claims that need fixture values rather than chart-plan behavior. A
// relationship becomes a coverage witness only when every member is present;
// partial diagnostic fixtures remain valid but cannot discharge coverage.
func (c *CompiledSemanticCase) ReconcileProductionClaims(
	ctx context.Context,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	if err := checkSemanticContext(ctx, "before production semantic-claim reconciliation"); err != nil {
		return err
	}
	if c == nil || c.root == nil || snapshot == nil || reconciled == nil {
		return fmt.Errorf("production semantic-claim reconciliation requires a compiled case, snapshot, and source join")
	}
	if err := c.reconcileOptionalIdentityConsistency(snapshot, reconciled); err != nil {
		return err
	}
	reconciled.Claims = nil
	for _, profile := range c.ActiveProfiles() {
		program := c.programs[profile]
		assignment := c.assignments[profile]
		for _, id := range sortedMapKeys(program.stateEncodings) {
			encoding := program.stateEncodings[id]
			if !encoding.availability.evaluate(program.environment.axes, assignment) {
				continue
			}
			observed, err := reconcileStateEncoding(program, encoding, snapshot, reconciled)
			if err != nil {
				return fmt.Errorf("profile %q state encoding %q: %w", profile, id, err)
			}
			if observed {
				reconciled.Claims = append(reconciled.Claims, ReconciledSemanticClaim{
					Profile: profile, Kind: "state_encoding", ID: id,
				})
			}
		}
		for _, id := range sortedMapKeys(program.relationships) {
			relationship := program.relationships[id]
			if !relationship.availability.evaluate(program.environment.axes, assignment) {
				continue
			}
			observed, err := reconcileRelationship(program, relationship, snapshot, reconciled)
			if err != nil {
				return fmt.Errorf("profile %q relationship %q: %w", profile, id, err)
			}
			if observed {
				reconciled.Claims = append(reconciled.Claims, ReconciledSemanticClaim{
					Profile: profile, Kind: "relationship", ID: id,
				})
			}
		}
	}
	slices.SortFunc(reconciled.Claims, func(left, right ReconciledSemanticClaim) int {
		if result := strings.Compare(left.Profile, right.Profile); result != 0 {
			return result
		}
		if result := strings.Compare(left.Kind, right.Kind); result != 0 {
			return result
		}
		return strings.Compare(left.ID, right.ID)
	})
	return nil
}

func (c *CompiledSemanticCase) reconcileOptionalIdentityConsistency(
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) error {
	type presenceState struct {
		present bool
		absent  bool
	}
	states := make(map[string]presenceState)
	for _, edge := range reconciled.Edges {
		if edge.SourceIndex < 0 || edge.SourceIndex >= len(snapshot.Sources) {
			return fmt.Errorf("optional identity consistency has invalid source index %d", edge.SourceIndex)
		}
		program := c.programs[edge.DestinationProfile]
		if program == nil || program.views[edge.View] == nil {
			return fmt.Errorf("optional identity consistency references unknown destination %s/%s",
				edge.DestinationProfile, edge.View)
		}
		labels := semanticPipelineLabelMap(snapshot.Sources[edge.SourceIndex].FinalLabels)
		for _, label := range program.views[edge.View].entity.Identity.Optional {
			key := edge.DestinationProfile + "\x00" + edge.View + "\x00" + label
			state := states[key]
			if strings.TrimSpace(labels[label]) == "" {
				state.absent = true
			} else {
				state.present = true
			}
			if state.present && state.absent {
				return fmt.Errorf(
					"profile %q view %q optional identity label %q is mixed present/absent within one proof case",
					edge.DestinationProfile, edge.View, label,
				)
			}
			states[key] = state
		}
	}
	return nil
}

type stateSeriesGroup struct {
	values map[string]float64
}

func reconcileStateEncoding(
	program *CompiledSemanticContract,
	encoding *compiledStateEncoding,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) (bool, error) {
	occurrences := make(map[string]struct{}, len(encoding.occurrences))
	for _, key := range encoding.occurrences {
		occurrences[key] = struct{}{}
	}
	groups := make(map[string]*stateSeriesGroup)
	for _, binding := range reconciled.Sources {
		if binding.program != program || binding.occurrence == nil || binding.SourceIndex < 0 ||
			binding.SourceIndex >= len(snapshot.Sources) {
			continue
		}
		if _, ok := occurrences[binding.occurrence.key]; !ok {
			continue
		}
		source := snapshot.Sources[binding.SourceIndex]
		labels := semanticLabelMap(source.Labels)
		state, ok := labels[encoding.definition.Label]
		if !ok || state == "" {
			return false, fmt.Errorf("source occurrence %q has no state label %q",
				source.OccurrenceID, encoding.definition.Label)
		}
		delete(labels, encoding.definition.Label)
		delete(labels, replayStructuralLabel(source.Component))
		key := binding.occurrence.key + "\x00" + canonicalLabelValueMap(labels)
		group := groups[key]
		if group == nil {
			group = &stateSeriesGroup{values: make(map[string]float64)}
			groups[key] = group
		}
		if _, duplicate := group.values[state]; duplicate {
			return false, fmt.Errorf("identity group %q duplicates state %q", key, state)
		}
		group.values[state] = source.Value
	}
	for key, group := range groups {
		ones := 0
		for _, state := range encoding.definition.States {
			value, ok := group.values[state]
			if !ok {
				return false, fmt.Errorf("identity group %q is missing state %q", key, state)
			}
			if value != 0 && value != 1 {
				return false, fmt.Errorf("identity group %q state %q has value %g, want 0 or 1", key, state, value)
			}
			if value == 1 {
				ones++
			}
		}
		if len(group.values) != len(encoding.definition.States) {
			return false, fmt.Errorf("identity group %q has unexpected state values", key)
		}
		if encoding.definition.Encoding == "one_hot_exactly_one" && ones != 1 {
			return false, fmt.Errorf("identity group %q has %d active states, want exactly one", key, ones)
		}
	}
	return len(groups) != 0, nil
}

type relationshipMemberValues struct {
	values     []float64
	sources    map[int]struct{}
	byIdentity map[string][]float64
	byGroup    map[string][]float64
}

func reconcileRelationship(
	program *CompiledSemanticContract,
	relationship *compiledRelationship,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) (bool, error) {
	definition := relationship.definition
	references := relationshipReferences(definition)
	members := make([]relationshipMemberValues, 0, len(references))
	for _, reference := range references {
		member, err := relationshipValues(program, reference, definition.GroupBy, snapshot, reconciled)
		if err != nil {
			return false, fmt.Errorf("member %s: %w", canonicalSourceReference(reference), err)
		}
		if len(member.values) == 0 {
			return false, nil
		}
		for _, value := range member.values {
			if math.IsNaN(value) || math.IsInf(value, 0) {
				return false, fmt.Errorf("member %s contains non-finite value %g", canonicalSourceReference(reference), value)
			}
		}
		members = append(members, member)
	}
	switch definition.Kind {
	case "equivalent":
		left := members[0].byIdentity
		right := members[1].byIdentity
		identityKind := "identities"
		if len(definition.GroupBy) != 0 {
			left = members[0].byGroup
			right = members[1].byGroup
			identityKind = "projected identities"
		}
		if !equalRelationshipIdentities(left, right) {
			return false, fmt.Errorf(
				"equivalent members have values %v and %v for %s %v and %v",
				members[0].values,
				members[1].values,
				identityKind,
				left,
				right,
			)
		}
	case "sum_projection":
		if err := reconcileSumProjection(members[0].byGroup, members[1].byGroup); err != nil {
			return false, err
		}
	case "partition":
		parts := members[1:]
		seen := make(map[int]struct{})
		for _, part := range parts {
			for source := range part.sources {
				if _, duplicate := seen[source]; duplicate {
					return false, fmt.Errorf("partition parts share source occurrence index %d", source)
				}
				seen[source] = struct{}{}
			}
		}
		if !nearlyEqualSemantic(sumSemanticValues(members[0].values), sumRelationshipMembers(parts)) {
			return false, fmt.Errorf("partition whole value %g differs from parts value %g",
				sumSemanticValues(members[0].values), sumRelationshipMembers(parts))
		}
	case "subset":
		subset := sumSemanticValues(members[0].values)
		superset := sumSemanticValues(members[1].values)
		if subset > superset && !nearlyEqualSemantic(subset, superset) {
			return false, fmt.Errorf("subset value %g exceeds superset value %g", subset, superset)
		}
	case "overlap":
		// Co-occurrence is the executable fixture witness. Source evidence owns
		// the claim that the populations actually overlap.
	default:
		panic("validated relationship kind has no replay claim")
	}
	return true, nil
}

func reconcileSumProjection(coarse, fine map[string][]float64) error {
	if len(coarse) != len(fine) {
		return fmt.Errorf("sum_projection has %d coarse groups and %d fine groups", len(coarse), len(fine))
	}
	for group, coarseValues := range coarse {
		if len(coarseValues) != 1 {
			return fmt.Errorf("group %q has %d coarse values, want one", group, len(coarseValues))
		}
		fineValues, ok := fine[group]
		if !ok {
			return fmt.Errorf("coarse group %q has no fine values", group)
		}
		fineSum := sumSemanticValues(fineValues)
		if !nearlyEqualSemantic(coarseValues[0], fineSum) {
			return fmt.Errorf("group %q coarse value %g differs from fine sum %g", group, coarseValues[0], fineSum)
		}
	}
	return nil
}

func relationshipValues(
	program *CompiledSemanticContract,
	reference SourceReference,
	groupBy []string,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
) (relationshipMemberValues, error) {
	components := make(map[string]struct{}, len(reference.Components))
	for _, component := range reference.Components {
		components[component] = struct{}{}
	}
	result := relationshipMemberValues{
		sources:    make(map[int]struct{}),
		byIdentity: make(map[string][]float64),
	}
	if len(groupBy) != 0 {
		result.byGroup = make(map[string][]float64)
	}
	for _, binding := range reconciled.Sources {
		if binding.program != program || binding.Signal != reference.Signal || binding.SourceIndex < 0 ||
			binding.SourceIndex >= len(snapshot.Sources) {
			continue
		}
		if _, ok := components[binding.Component]; !ok {
			continue
		}
		source := snapshot.Sources[binding.SourceIndex]
		if !replayLabelConditionMatches(reference.Where, semanticLabelMap(source.Labels)) {
			continue
		}
		result.values = append(result.values, source.Value)
		result.sources[binding.SourceIndex] = struct{}{}
		labels := semanticLabelMap(source.Labels)
		delete(labels, replayStructuralLabel(source.Component))
		identity := canonicalLabelValueMap(labels)
		result.byIdentity[identity] = append(result.byIdentity[identity], source.Value)
		if len(groupBy) != 0 {
			groupLabels := make(map[string]string, len(groupBy))
			for _, label := range groupBy {
				if value, ok := labels[label]; ok && strings.TrimSpace(value) != "" {
					groupLabels[label] = value
				}
			}
			group := canonicalLabelValueMap(groupLabels)
			result.byGroup[group] = append(result.byGroup[group], source.Value)
		}
	}
	return result, nil
}

func equalRelationshipIdentities(left, right map[string][]float64) bool {
	if len(left) != len(right) {
		return false
	}
	for identity, leftValues := range left {
		rightValues, ok := right[identity]
		if !ok || !slices.Equal(sortedFloatBits(leftValues), sortedFloatBits(rightValues)) {
			return false
		}
	}
	return true
}

func canonicalLabelValueMap(labels map[string]string) string {
	pairs := make([]string, 0, len(labels))
	for _, name := range sortedMapKeys(labels) {
		pairs = append(pairs, name+"="+labels[name])
	}
	return strings.Join(pairs, "\x00")
}

func sortedFloatBits(values []float64) []uint64 {
	bits := make([]uint64, 0, len(values))
	for _, value := range values {
		bits = append(bits, math.Float64bits(value))
	}
	slices.Sort(bits)
	return bits
}

func sumSemanticValues(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func sumRelationshipMembers(members []relationshipMemberValues) float64 {
	total := 0.0
	for _, member := range members {
		total += sumSemanticValues(member.values)
	}
	return total
}

func nearlyEqualSemantic(left, right float64) bool {
	difference := math.Abs(left - right)
	scale := math.Max(1, math.Max(math.Abs(left), math.Abs(right)))
	return difference <= scale*1e-12
}
