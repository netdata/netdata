// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionObservationTracksPersistentTransitions(t *testing.T) {
	semanticCase, first, firstJoin := validProductionPlanCase(t)
	first.PlanActions = append(first.PlanActions, semanticUpdateAction(10, false))
	state, err := semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "current", Membership: "establish", Aggregate: "matches_reducer", Identity: "establish",
		},
		first,
		firstJoin,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	second := promreplay.CloneSemanticSnapshot(first)
	second.Sources[0].Value = 14
	second.PlanActions = []promreplay.SemanticPlanAction{semanticUpdateAction(14, false)}
	secondJoin := reconcileTestProductionCase(t, semanticCase, second)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), second, secondJoin); err != nil {
		t.Fatal(err)
	}
	state, err = semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "current", Membership: "unchanged", Aggregate: "increased", Identity: "unchanged",
		},
		second,
		secondJoin,
		state,
	)
	if err != nil {
		t.Fatal(err)
	}

	stale := promreplay.CloneSemanticSnapshot(second)
	stale.Sources = nil
	stale.PlanActions = []promreplay.SemanticPlanAction{semanticUpdateAction(0, true)}
	staleJoin := reconcileTestProductionCase(t, semanticCase, stale)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), stale, staleJoin); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "stale", Membership: "removed", Aggregate: "became_gap", Identity: "unchanged",
		},
		stale,
		staleJoin,
		state,
	); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileProductionObservationRecognizesRemovalAsAbsent(t *testing.T) {
	semanticCase, first, firstJoin := validProductionPlanCase(t)
	first.PlanActions = append(first.PlanActions, semanticUpdateAction(10, false))
	state, err := semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "current", Membership: "establish", Aggregate: "matches_reducer", Identity: "establish",
		},
		first,
		firstJoin,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	absent := promreplay.CloneSemanticSnapshot(first)
	absent.Sources = nil
	absent.PlanActions = []promreplay.SemanticPlanAction{{
		Kind: "remove_dimension", ChartID: "requests", DimensionName: "requests",
	}}
	absentJoin := reconcileTestProductionCase(t, semanticCase, absent)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), absent, absentJoin); err != nil {
		t.Fatal(err)
	}
	if _, err := semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "absent", Membership: "removed", Aggregate: "became_gap", Identity: "absent",
		},
		absent,
		absentJoin,
		state,
	); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileProductionObservationRejectsWrongAggregateDirection(t *testing.T) {
	semanticCase, first, firstJoin := validProductionPlanCase(t)
	first.PlanActions = append(first.PlanActions, semanticUpdateAction(10, false))
	state, err := semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "current", Membership: "establish", Aggregate: "matches_reducer", Identity: "establish",
		},
		first,
		firstJoin,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	second := promreplay.CloneSemanticSnapshot(first)
	second.PlanActions = []promreplay.SemanticPlanAction{semanticUpdateAction(9, false)}
	secondJoin := reconcileTestProductionCase(t, semanticCase, second)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), second, secondJoin); err != nil {
		t.Fatal(err)
	}
	_, err = semanticCase.ReconcileProductionObservation(
		context.Background(),
		"requests#requests",
		ProductionObservationExpectation{
			State: "current", Membership: "unchanged", Aggregate: "increased", Identity: "unchanged",
		},
		second,
		secondJoin,
		state,
	)
	if err == nil || !strings.Contains(err.Error(), "decreased") {
		t.Fatalf("ReconcileProductionObservation() error = %v, want decreased mismatch", err)
	}
}

func TestProductionObservationMemberAllowsAbsentOptionalContributorIdentity(t *testing.T) {
	contract := loadTestSemanticContract(
		t,
		embeddedWorkerReductionDesignV1(),
		sourceWithEmbeddedWorkerContributorsV1(),
		registryWithSingleOperationV1(),
		validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	occurrence := program.occurrences[program.signals["requests"].occurrences[0]]
	binding := ReconciledSemanticSource{
		Profile: "example", Signal: "requests", Component: "total", program: program, occurrence: occurrence,
	}

	canonical, err := semanticCase.productionObservationMember(
		binding,
		promreplay.SemanticSource{OccurrenceID: "canonical"},
	)
	if err != nil {
		t.Fatalf("canonical contributor: %v", err)
	}
	embedded, err := semanticCase.productionObservationMember(
		binding,
		promreplay.SemanticSource{
			OccurrenceID: "embedded",
			FinalLabels:  []promreplay.SemanticLabel{{Name: "worker", Value: "worker-a"}},
		},
	)
	if err != nil {
		t.Fatalf("embedded contributor: %v", err)
	}
	if canonical == embedded {
		t.Fatalf("canonical and embedded contributor identities both encoded as %q", canonical)
	}
}

func TestProductionObservationMemberRejectsAbsentRequiredContributorIdentity(t *testing.T) {
	program := compileTestSemanticContract(t, workerAverageReductionDesignV1(), sourceWithWorkerContributorsV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	occurrence := program.occurrences[program.signals["requests"].occurrences[0]]
	binding := ReconciledSemanticSource{
		Profile: "example", Signal: "requests", Component: "total", program: program, occurrence: occurrence,
	}

	_, err = semanticCase.productionObservationMember(binding, promreplay.SemanticSource{OccurrenceID: "missing"})
	if err == nil || !strings.Contains(err.Error(), `contributor identity label "pid" is absent or blank`) {
		t.Fatalf("productionObservationMember() error = %v, want required contributor identity failure", err)
	}
}

func semanticUpdateAction(value float64, empty bool) promreplay.SemanticPlanAction {
	return promreplay.SemanticPlanAction{
		Kind: "update_dimension", ChartID: "requests", DimensionName: "requests",
		Float: true, Float64: value, IsEmpty: empty,
	}
}
