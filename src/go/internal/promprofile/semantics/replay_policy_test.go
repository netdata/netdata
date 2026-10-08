// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionChartPoliciesAcceptsDerivedDefaults(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}

	if err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionChartPolicies() error = %v", err)
	}
}

func TestReconcileProductionChartPoliciesRequiresAlgorithmWhenWireKindDiffersFromLifecycle(t *testing.T) {
	source := strings.Replace(
		validSourceSemanticsV1,
		"prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: gauge, shape: scalar}",
		1,
	)
	program := compileTestSemanticContract(t, validProfileDesignV1, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	snapshot.Sources[0].PrometheusType = "gauge"
	snapshot.Sources[0].Routes[0].SeriesKind = "gauge"
	snapshot.Profiles[0].Charts[0].DeclaredAlgorithm = "incremental"
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}

	if err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionChartPolicies() error = %v", err)
	}
}

func TestReconcileProductionChartPoliciesAcceptsExplicitPriority(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}

	snapshot := validProductionRouteSnapshot()
	snapshot.Profiles[0].Charts[0].Priority = 100
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("explicit chart priority must be accepted: %v", err)
	}
}

func TestReconcileProductionChartPoliciesRejectsStaticDebt(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		mutate func(*promreplay.SemanticChartPolicy)
		want   string
	}{
		"explicit id": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.ExplicitID = "requests" },
			want:   "redundant explicit ID",
		},
		"wildcard identity": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.WildcardIdentity = true },
			want:   "wildcard all-label identity",
		},
		"lifecycle cap": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.MaxInstances = 10 },
			want:   "lifecycle caps/expiry",
		},
		"redundant aggregation": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.DeclaredAggregation = "sum" },
			want:   `aggregation declaration got "sum", want ""`,
		},
		"redundant algorithm": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.DeclaredAlgorithm = "incremental" },
			want:   `algorithm declaration got "incremental", want ""`,
		},
		"redundant type": {
			mutate: func(policy *promreplay.SemanticChartPolicy) { policy.DeclaredType = "line" },
			want:   `type declaration got "line", want ""`,
		},
		"redundant scale": {
			mutate: func(policy *promreplay.SemanticChartPolicy) {
				policy.Dimensions[0].ExplicitMultiplier = 1
			},
			want: "scale declaration",
		},
		"redundant float": {
			mutate: func(policy *promreplay.SemanticChartPolicy) {
				policy.Dimensions[0].ExplicitFloat = true
			},
			want: "redundant float storage",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			snapshot := validProductionRouteSnapshot()
			tc.mutate(&snapshot.Profiles[0].Charts[0])
			reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
			if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
				t.Fatal(err)
			}
			err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ReconcileProductionChartPolicies() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestReconcileProductionChartPoliciesRequiresExplicitReducer(t *testing.T) {
	program := compileTestSemanticContract(t, workerAverageReductionDesignV1(), sourceWithWorkerContributorsV1())
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionRouteSnapshot()
	source := &snapshot.Sources[0]
	source.Labels = semanticTestLabels("pid", "100")
	source.FinalLabels = semanticTestLabels("pid", "100")
	source.PrometheusType = "gauge"
	route := &source.Routes[0]
	route.PromotionMode = "identity_only"
	route.Algorithm = "absolute"
	route.SeriesKind = "gauge"
	route.Aggregation = "avg"
	route.Units = "requests"
	snapshot.Profiles[0].Charts[0].DeclaredAggregation = "avg"
	reconciled := reconcileTestProductionCase(t, semanticCase, snapshot)
	if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionChartPolicies() error = %v", err)
	}

	snapshot.Profiles[0].Charts[0].DeclaredAggregation = ""
	err = semanticCase.ReconcileProductionChartPolicies(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), `aggregation declaration got "", want "avg"`) {
		t.Fatalf("ReconcileProductionChartPolicies() error = %v, want missing reducer", err)
	}
}
