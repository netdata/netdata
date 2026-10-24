// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionPlanValidatesAuthoredPublicIdentities(t *testing.T) {
	semanticCase, snapshot, reconciled := validProductionPlanCase(t)

	if err := semanticCase.ReconcileProductionPlan(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionPlan() error = %v", err)
	}
}

func TestReconcileProductionPlanAcceptsExistingChartLabelUpdate(t *testing.T) {
	semanticCase, snapshot, reconciled := validProductionPlanCase(t)
	snapshot.PlanActions[0].Kind = "update_chart_labels"
	snapshot.PlanActions[0].ChartTemplateID = ""

	if err := semanticCase.ReconcileProductionPlan(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionPlan() error = %v", err)
	}
}

func TestReconcileProductionPlanRejectsDefinitionAndIdentityDrift(t *testing.T) {
	tests := map[string]struct {
		mutate func(*promreplay.SemanticSnapshot)
		want   string
	}{
		"empty public chart identity": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.PlanActions[0].WireChartID = "" },
			want:   "empty internal or public chart identity",
		},
		"wrong chart template": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.PlanActions[0].ChartTemplateID = "wrong" },
			want:   "chart template ID",
		},
		"wrong chart labels": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.PlanActions[0].Labels = semanticTestLabels("unexpected", "value")
			},
			want: "chart labels",
		},
		"wrong dimension algorithm": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.PlanActions[1].Algorithm = "absolute" },
			want:   "algorithm",
		},
		"missing writer float": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) { snapshot.PlanActions[1].Float = false },
			want:   "did not inherit writer float storage",
		},
		"unexpected authored dimension": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.PlanActions[1].DimensionName = "unexpected"
				snapshot.PlanActions[1].WireDimensionID = "unexpected"
			},
			want: "unexpected authored dimension",
		},
		"public chart collision": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.PlanActions = append(snapshot.PlanActions, promreplay.SemanticPlanAction{
					Kind: "create_chart", ChartID: "other", Context: "prometheus.example.other",
					WireTypeID: "prometheus.example", WireChartID: "requests", WireContext: "prometheus.example.other",
				})
			},
			want: "public chart identity collides",
		},
		"public context collision": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.PlanActions = append(snapshot.PlanActions, promreplay.SemanticPlanAction{
					Kind: "create_chart", ChartID: "other", Context: "prometheus.example.other",
					WireTypeID: "prometheus.example", WireChartID: "other", WireContext: "prometheus.example.requests",
				})
			},
			want: "public context",
		},
		"public dimension collision": {
			mutate: func(snapshot *promreplay.SemanticSnapshot) {
				snapshot.PlanActions = append(snapshot.PlanActions, promreplay.SemanticPlanAction{
					Kind: "create_dimension", ChartID: "requests", DimensionName: "other",
					WireTypeID: "prometheus.example", WireChartID: "requests", WireDimensionID: "requests",
				})
			},
			want: "public dimension identity collides",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			semanticCase, snapshot, reconciled := validProductionPlanCase(t)
			tc.mutate(snapshot)
			err := semanticCase.ReconcileProductionPlan(context.Background(), snapshot, reconciled)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReconcileProductionPlan() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func validProductionPlanCase(
	t *testing.T,
) (*CompiledSemanticCase, *promreplay.SemanticSnapshot, *ReconciledSemanticCase) {
	t.Helper()
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.PlanActions = []promreplay.SemanticPlanAction{
		{
			Kind: "create_chart", ChartTemplateID: "template-0", ChartID: "requests",
			Context: "prometheus.example.requests", DisplayedFamily: "Traffic/Requests",
			Units: "requests/s", Presentation: "line",
			WireTypeID: "prometheus.example", WireChartID: "requests", WireContext: "prometheus.example.requests",
		},
		{
			Kind: "create_dimension", ChartID: "requests", DimensionName: "requests",
			Context: "prometheus.example.requests", DisplayedFamily: "Traffic/Requests",
			Units: "requests/s", Presentation: "line", Algorithm: "incremental", Float: true,
			Multiplier: 1, Divisor: 1,
			WireTypeID: "prometheus.example", WireChartID: "requests", WireDimensionID: "requests",
		},
	}
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	return semanticCase, snapshot, reconciled
}
