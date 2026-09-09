// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

// ProductionObservationExpectation is the closed proof predicate vocabulary
// for one semantic view/input target at one persistent replay step.
type ProductionObservationExpectation struct {
	State      string
	Membership string
	Aggregate  string
	Identity   string
}

// ProductionObservationState is the bounded previous-target state retained
// between steps of one persistent proof case.
type ProductionObservationState struct {
	state       string
	members     map[string]struct{}
	identities  map[string]struct{}
	aggregates  map[string]productionObservationValue
	runtimeKeys map[productionObservationRuntimeKey]string
}

type productionObservationValue struct {
	gap   bool
	value float64
}

type productionObservationRuntimeKey struct {
	chart     string
	dimension string
}

// ReconcileProductionObservation derives the target state from production
// routes and plan actions, then checks the authored transition predicates.
func (c *CompiledSemanticCase) ReconcileProductionObservation(
	ctx context.Context,
	target string,
	expected ProductionObservationExpectation,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
	previous *ProductionObservationState,
) (*ProductionObservationState, error) {
	if err := checkSemanticContext(ctx, "before production observation reconciliation"); err != nil {
		return nil, err
	}
	if c == nil || c.root == nil || snapshot == nil || reconciled == nil {
		return nil, fmt.Errorf("production observation reconciliation: case, snapshot, and source join must be present")
	}
	if err := c.ValidateObservationTarget(target, ""); err != nil {
		return nil, fmt.Errorf("production observation reconciliation: %w", err)
	}
	current, err := c.productionObservationState(target, snapshot, reconciled, previous)
	if err != nil {
		return nil, fmt.Errorf("production observation %q: %w", target, err)
	}
	if err := verifyProductionObservation(expected, previous, current); err != nil {
		return nil, fmt.Errorf("production observation %q: %w", target, err)
	}
	return current, nil
}

func (c *CompiledSemanticCase) productionObservationState(
	target string,
	snapshot *promreplay.SemanticSnapshot,
	reconciled *ReconciledSemanticCase,
	previous *ProductionObservationState,
) (*ProductionObservationState, error) {
	viewID, inputID, _ := strings.Cut(target, "#")
	state := &ProductionObservationState{
		members:     make(map[string]struct{}),
		identities:  make(map[string]struct{}),
		aggregates:  make(map[string]productionObservationValue),
		runtimeKeys: make(map[productionObservationRuntimeKey]string),
	}
	updates := make(map[productionObservationRuntimeKey]productionObservationValue)
	removedCharts := make(map[string]struct{})
	removedDimensions := make(map[productionObservationRuntimeKey]struct{})
	for _, action := range snapshot.PlanActions {
		key := productionObservationRuntimeKey{chart: action.ChartID, dimension: action.DimensionName}
		switch action.Kind {
		case "update_dimension":
			if _, exists := updates[key]; exists {
				return nil, fmt.Errorf("production plan duplicates update for chart %q dimension %q", key.chart, key.dimension)
			}
			value := productionObservationValue{gap: action.IsEmpty}
			if !action.IsEmpty {
				if action.Float {
					value.value = action.Float64
				} else {
					value.value = float64(action.Int64)
				}
			}
			updates[key] = value
		case "remove_chart":
			removedCharts[action.ChartID] = struct{}{}
		case "remove_dimension":
			removedDimensions[key] = struct{}{}
		}
	}

	for _, edge := range reconciled.Edges {
		if edge.DestinationProfile != c.root.profile || edge.View != viewID || edge.Input != inputID {
			continue
		}
		if edge.SourceIndex < 0 || edge.SourceIndex >= len(snapshot.Sources) {
			return nil, fmt.Errorf("semantic edge has invalid source index %d", edge.SourceIndex)
		}
		source := snapshot.Sources[edge.SourceIndex]
		route, err := productionObservationRoute(edge, source.Routes)
		if err != nil {
			return nil, err
		}
		identity, err := productionObservationIdentity(edge, route)
		if err != nil {
			return nil, err
		}
		key := productionObservationRuntimeKey{chart: edge.ChartID, dimension: edge.DimensionName}
		if prior, exists := state.runtimeKeys[key]; exists && prior != identity {
			return nil, fmt.Errorf("runtime output %q/%q maps to semantic identities %q and %q",
				key.chart, key.dimension, prior, identity)
		}
		state.runtimeKeys[key] = identity
		state.identities[identity] = struct{}{}
		member, err := c.productionObservationMember(reconciled.Sources[edge.SourceIndex], source)
		if err != nil {
			return nil, err
		}
		state.members[identity+"\x00"+member] = struct{}{}
	}

	if len(state.runtimeKeys) != 0 {
		for key, identity := range state.runtimeKeys {
			value, ok := updates[key]
			if !ok {
				return nil, fmt.Errorf("current output %q/%q has no production value update", key.chart, key.dimension)
			}
			if value.gap {
				return nil, fmt.Errorf("current output %q/%q emitted a gap", key.chart, key.dimension)
			}
			if prior, exists := state.aggregates[identity]; exists && !sameObservationValue(prior, value) {
				return nil, fmt.Errorf("semantic identity %q has conflicting production aggregate values", identity)
			}
			state.aggregates[identity] = value
		}
		state.state = "current"
		return state, nil
	}

	if previous == nil || len(previous.runtimeKeys) == 0 {
		state.state = "absent"
		return state, nil
	}
	for key, identity := range previous.runtimeKeys {
		_, chartRemoved := removedCharts[key.chart]
		_, dimensionRemoved := removedDimensions[key]
		if chartRemoved || dimensionRemoved {
			state.aggregates[identity] = productionObservationValue{gap: true}
			continue
		}
		if value, updated := updates[key]; updated && !value.gap {
			return nil, fmt.Errorf("absent target output %q/%q still emitted a non-gap value", key.chart, key.dimension)
		}
		state.runtimeKeys[key] = identity
		state.identities[identity] = struct{}{}
		state.aggregates[identity] = productionObservationValue{gap: true}
	}
	if len(state.runtimeKeys) == 0 {
		state.state = "absent"
	} else {
		state.state = "stale"
	}
	return state, nil
}

func productionObservationRoute(
	edge ReconciledSemanticEdge,
	routes []promreplay.SemanticRoute,
) (promreplay.SemanticRoute, error) {
	var matches []promreplay.SemanticRoute
	for _, route := range routes {
		if route.Profile == edge.DestinationProfile && route.TemplatePath == edge.TemplatePath &&
			route.ChartID == edge.ChartID && route.DimensionIndex == edge.DimensionIndex &&
			route.DimensionName == edge.DimensionName {
			matches = append(matches, route)
		}
	}
	if len(matches) != 1 {
		return promreplay.SemanticRoute{}, fmt.Errorf(
			"semantic edge %s/%s#%s resolves to %d production routes", edge.DestinationProfile, edge.View, edge.Input, len(matches))
	}
	return matches[0], nil
}

func productionObservationIdentity(
	edge ReconciledSemanticEdge,
	route promreplay.SemanticRoute,
) (string, error) {
	values := make(map[string]string, len(route.ChartLabelValues))
	for _, label := range route.ChartLabelValues {
		values[label.Name] = label.Value
	}
	parts := []string{edge.DestinationProfile, edge.View, edge.RenderedRole}
	for _, label := range route.IdentityLabels {
		value := values[label]
		if value == "" {
			return "", fmt.Errorf("semantic identity label %q is absent or blank", label)
		}
		parts = append(parts, label, value)
	}
	parts = append(parts, "dimension", edge.DimensionName)
	return encodeObservationParts(parts...), nil
}

func (c *CompiledSemanticCase) productionObservationMember(
	binding ReconciledSemanticSource,
	source promreplay.SemanticSource,
) (string, error) {
	signal := binding.program.signals[binding.Signal]
	if len(signal.contributors) == 0 {
		return encodeObservationParts(binding.Profile, binding.Signal, binding.Component, source.OccurrenceID), nil
	}
	assignment := c.assignments[binding.Profile]
	var active []compiledContributorVariant
	for _, variant := range signal.contributors {
		if variant.availability.evaluate(binding.program.environment.axes, assignment) {
			active = append(active, variant)
		}
	}
	if len(active) != 1 {
		return "", fmt.Errorf("source %s/%s has %d active contributor variants", binding.Profile, binding.Signal, len(active))
	}
	labels := semanticPipelineLabelMap(source.FinalLabels)
	parts := []string{binding.Profile, binding.Signal, binding.Component, active[0].id}
	for _, label := range active[0].definition.Identity {
		normalized := normalizedOccurrenceIdentityLabel(binding.occurrence, label)
		value := labels[normalized]
		if value == "" {
			schema := binding.occurrence.labels[normalized]
			optional, err := contributorIdentityOptional(binding.program, schema, assignment)
			if err != nil {
				return "", fmt.Errorf("source %s/%s contributor identity label %q: %w",
					binding.Profile, binding.Signal, label, err)
			}
			if optional {
				continue
			}
			return "", fmt.Errorf("source %s/%s contributor identity label %q is absent or blank",
				binding.Profile, binding.Signal, label)
		}
		parts = append(parts, label, value)
	}
	return encodeObservationParts(parts...), nil
}

func contributorIdentityOptional(
	program *CompiledSemanticContract,
	schema SourceLabel,
	assignment map[string]AxisValue,
) (bool, error) {
	switch schema.Presence.Kind {
	case "required", "present":
		return false, nil
	case "optional":
		return true, nil
	default:
		presence, err := program.environment.resolve(schema.Presence.When)
		if err != nil {
			return false, err
		}
		return !presence.evaluate(program.environment.axes, assignment), nil
	}
}

func verifyProductionObservation(
	expected ProductionObservationExpectation,
	previous, current *ProductionObservationState,
) error {
	if current.state != expected.State {
		return fmt.Errorf("state got %q, want %q", current.state, expected.State)
	}
	if err := verifyObservationMembership(expected.Membership, previous, current); err != nil {
		return fmt.Errorf("membership %q: %w", expected.Membership, err)
	}
	if err := verifyObservationAggregate(expected.Aggregate, previous, current); err != nil {
		return fmt.Errorf("aggregate %q: %w", expected.Aggregate, err)
	}
	if err := verifyObservationIdentity(expected.Identity, previous, current); err != nil {
		return fmt.Errorf("identity %q: %w", expected.Identity, err)
	}
	return nil
}

func verifyObservationMembership(
	predicate string,
	previous, current *ProductionObservationState,
) error {
	if predicate == "establish" {
		if previous != nil {
			return fmt.Errorf("requires no previous observation")
		}
		if len(current.members) == 0 {
			return fmt.Errorf("has no current contributors")
		}
		return nil
	}
	if previous == nil {
		return fmt.Errorf("requires a previous observation")
	}
	removed := setDifferenceCount(previous.members, current.members)
	added := setDifferenceCount(current.members, previous.members)
	switch predicate {
	case "unchanged":
		if removed != 0 || added != 0 {
			return fmt.Errorf("removed=%d added=%d", removed, added)
		}
	case "added":
		if removed != 0 || added == 0 {
			return fmt.Errorf("removed=%d added=%d", removed, added)
		}
	case "removed":
		if removed == 0 || added != 0 {
			return fmt.Errorf("removed=%d added=%d", removed, added)
		}
	case "replaced":
		if removed == 0 || added == 0 {
			return fmt.Errorf("removed=%d added=%d", removed, added)
		}
	default:
		return fmt.Errorf("unknown predicate")
	}
	return nil
}

func verifyObservationAggregate(
	predicate string,
	previous, current *ProductionObservationState,
) error {
	if predicate == "matches_reducer" {
		if previous != nil {
			return fmt.Errorf("requires no previous observation")
		}
		if len(current.aggregates) == 0 {
			return fmt.Errorf("has no production aggregates")
		}
		for _, value := range current.aggregates {
			if value.gap {
				return fmt.Errorf("production aggregate is a gap")
			}
		}
		return nil
	}
	if previous == nil {
		return fmt.Errorf("requires a previous observation")
	}
	switch predicate {
	case "unchanged":
		if !sameObservationValues(previous.aggregates, current.aggregates) {
			return fmt.Errorf("production aggregate set changed")
		}
	case "increased", "decreased":
		if !sameStringSet(mapKeysObservationValues(previous.aggregates), mapKeysObservationValues(current.aggregates)) {
			return fmt.Errorf("production aggregate identities changed")
		}
		strict := false
		for identity, before := range previous.aggregates {
			after := current.aggregates[identity]
			if before.gap || after.gap || math.IsNaN(before.value) || math.IsNaN(after.value) ||
				math.IsInf(before.value, 0) || math.IsInf(after.value, 0) {
				return fmt.Errorf("identity %q is not a finite transition", identity)
			}
			if predicate == "increased" {
				if after.value < before.value {
					return fmt.Errorf("identity %q decreased", identity)
				}
				strict = strict || after.value > before.value
			} else {
				if after.value > before.value {
					return fmt.Errorf("identity %q increased", identity)
				}
				strict = strict || after.value < before.value
			}
		}
		if !strict {
			return fmt.Errorf("no aggregate changed")
		}
	case "became_gap":
		becameGap := false
		for identity, before := range previous.aggregates {
			after, ok := current.aggregates[identity]
			if ok && !before.gap && after.gap {
				becameGap = true
			}
		}
		if !becameGap {
			return fmt.Errorf("no retained aggregate became a gap")
		}
	default:
		return fmt.Errorf("unknown predicate")
	}
	return nil
}

func verifyObservationIdentity(
	predicate string,
	previous, current *ProductionObservationState,
) error {
	if predicate == "establish" {
		if previous != nil {
			return fmt.Errorf("requires no previous observation")
		}
		if len(current.identities) == 0 {
			return fmt.Errorf("has no semantic/public identity")
		}
		return nil
	}
	if previous == nil {
		return fmt.Errorf("requires a previous observation")
	}
	equal := sameStringSet(previous.identities, current.identities)
	switch predicate {
	case "unchanged":
		if !equal {
			return fmt.Errorf("semantic/public identity set changed")
		}
	case "changed":
		if equal || len(current.identities) == 0 {
			return fmt.Errorf("semantic/public identity set did not change to a present identity")
		}
	case "absent":
		if len(current.identities) != 0 {
			return fmt.Errorf("semantic/public identities remain present")
		}
	default:
		return fmt.Errorf("unknown predicate")
	}
	return nil
}

func encodeObservationParts(parts ...string) string {
	var builder strings.Builder
	for _, part := range parts {
		fmt.Fprintf(&builder, "%d:%s", len(part), part)
	}
	return builder.String()
}

func setDifferenceCount(left, right map[string]struct{}) int {
	count := 0
	for value := range left {
		if _, ok := right[value]; !ok {
			count++
		}
	}
	return count
}

func sameStringSet(left, right map[string]struct{}) bool {
	return len(left) == len(right) && setDifferenceCount(left, right) == 0
}

func mapKeysObservationValues(values map[string]productionObservationValue) map[string]struct{} {
	keys := make(map[string]struct{}, len(values))
	for key := range values {
		keys[key] = struct{}{}
	}
	return keys
}

func sameObservationValues(
	left, right map[string]productionObservationValue,
) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		other, ok := right[key]
		if !ok || !sameObservationValue(value, other) {
			return false
		}
	}
	return true
}

func sameObservationValue(left, right productionObservationValue) bool {
	return left.gap == right.gap && (left.gap || left.value == right.value ||
		(math.IsNaN(left.value) && math.IsNaN(right.value)))
}
