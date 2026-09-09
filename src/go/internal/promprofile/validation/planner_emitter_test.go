// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
)

func TestValidateProfileFindsDeadChart(t *testing.T) {
	profile := strings.Replace(
		validProfile,
		"    - title: Temperature",
		"    - title: Missing\n      context: missing\n      units: values\n      priority: 90\n      dimensions:\n        - selector: app_missing\n          name: missing\n    - title: Temperature",
		1,
	)
	profile = strings.Replace(profile, "  metrics:\n", "  metrics:\n    - app_missing\n", 1)
	result := runValidation(t, profile, validDump, "")
	requireFinding(t, result, "dead_chart")
}

func TestValidateProfileFindsDeadDimensionInsideLiveChart(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_live]
  charts:
    - title: Mixed
      context: mixed
      units: values
      dimensions:
        - selector: app_live
          name: live
        - selector: app_live
          name_from_label: missing_label
`
	result := runValidation(t, profile, "# TYPE app_live gauge\napp_live 1\n", "")
	requireFinding(t, result, "dead_dimension")
	if len(result.report.DeadCharts) != 0 || len(result.report.DeadDimensions) != 1 {
		t.Fatalf("expected one dead dimension in a live chart: charts=%#v dimensions=%#v", result.report.DeadCharts, result.report.DeadDimensions)
	}
}

func TestValidateProfileFindsMissingExplicitInstanceIdentity(t *testing.T) {
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "by_labels: [instance]", "by_labels: [node]")
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value{instance=\"a\"} 1\n", "")
	requireFinding(t, result, "dead_chart")
	if len(result.report.DeadCharts) != 1 {
		t.Fatalf("missing explicit identity label should make the chart unroutable: %#v", result.report.DeadCharts)
	}
}

func TestValidateProfileFindsObservedSameTemplateInstanceIDCollision(t *testing.T) {
	dump := "# TYPE app_value gauge\napp_value{instance=\"a.b\"} 1\napp_value{instance=\"a_b\"} 2\n"
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, "")
	requireFinding(t, result, "instance_id_collision_observed")
	if len(result.report.InstanceLosses) != 1 {
		t.Fatalf("expected one observed instance materialization loss: %#v", result.report.InstanceLosses)
	}
	if result.report.InstanceLosses[0].ObservedIdentities != 2 || result.report.InstanceLosses[0].RenderedIDs != 1 {
		t.Fatalf("unexpected instance collision counts: %#v", result.report.InstanceLosses[0])
	}
	if strings.Contains(result.stdout, "a.b") || strings.Contains(result.stdout, "a_b") {
		t.Fatalf("report leaked label-derived instance values:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsLifecycleInstanceMaterializationLoss(t *testing.T) {
	profile := strings.Replace(
		singleInstanceValueGaugeProfile,
		"      instances:\n        by_labels: [instance]\n",
		"      instances:\n        by_labels: [instance]\n      lifecycle:\n        max_instances: 1\n",
		1,
	)
	dump := "# TYPE app_value gauge\napp_value{instance=\"a\"} 1\napp_value{instance=\"b\"} 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "instance_materialization_loss_observed")
	if len(result.report.InstanceLosses) != 1 ||
		result.report.InstanceLosses[0].ObservedIdentities != 2 ||
		result.report.InstanceLosses[0].RenderedIDs != 1 ||
		result.report.InstanceLosses[0].Cause != "lifecycle_limit_or_rendered_id_collapse" {
		t.Fatalf("unexpected lifecycle instance loss report: %#v", result.report.InstanceLosses)
	}
}

func TestValidateProfileFindsObservedDimensionWireIDCollision(t *testing.T) {
	dump := "# TYPE app_value gauge\napp_value{state=\"a\"} 1\napp_value{state=\"'a\"} 2\n"
	result := runValidation(t, singleDynamicValueGaugeProfile, dump, "")
	requireFinding(t, result, "dimension_id_collision_observed")
	if len(result.report.DimensionCollisions) != 1 {
		t.Fatalf("expected one emitted dimension ID collision: %#v", result.report.DimensionCollisions)
	}
	if strings.Contains(result.stdout, "\"'a\"") {
		t.Fatalf("report leaked label-derived dimension value:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsDimensionLostAtWireEmission(t *testing.T) {
	result := runValidation(t, singleDynamicValueGaugeProfile, "# TYPE app_value gauge\napp_value{state=\"'\"} 1\n", "")
	requireFinding(t, result, "dimension_wire_emission_loss")
	if strings.Contains(result.stdout, "state=\\\"'\\\"") {
		t.Fatalf("report leaked label-derived dimension value:\n%s", result.stdout)
	}
}

func TestValidateProfileFindsObservedChartWireIDCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: one
      units: values
      dimensions:
        - selector: app_one
          name: one
    - id: "'one"
      title: Two
      context: two
      units: values
      dimensions:
        - selector: app_two
          name: two
`
	result := runValidation(t, profile, twoValueGaugesDump, "")
	requireFinding(t, result, "chart_wire_id_collision_observed")
	if len(result.report.ChartWireCollisions) != 1 ||
		result.report.ChartWireCollisions[0].Occurrences != 2 {
		t.Fatalf("expected one two-chart wire collision: %#v", result.report.ChartWireCollisions)
	}
}

func TestValidateProfileFindsObservedContextWireCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: shared
      units: values
      dimensions:
        - selector: app_one
          name: one
    - id: two
      title: Two
      context: "'shared"
      units: values
      dimensions:
        - selector: app_two
          name: two
`
	result := runValidation(t, profile, twoValueGaugesDump, "")
	requireFinding(t, result, "context_wire_collision_observed")
	if len(result.report.ContextCollisions) != 1 ||
		len(result.report.ContextCollisions[0].RawContextFingerprints) != 2 {
		t.Fatalf("expected two distinct raw contexts to collapse: %#v", result.report.ContextCollisions)
	}
}

func TestInspectEmittedPlanAssociatesContextsInEmitterOrder(t *testing.T) {
	plan := chartengine.Plan{Actions: []chartengine.EngineAction{
		chartengine.CreateChartAction{ChartID: "z", Meta: chartengine.ChartMeta{Context: "'shared"}},
		chartengine.CreateChartAction{ChartID: "a", Meta: chartengine.ChartMeta{Context: "shared"}},
		chartengine.CreateChartAction{ChartID: "m", Meta: chartengine.ChartMeta{Context: "distinct"}},
	}}

	result, err := inspectEmittedPlan(plan, collectorJobFullName("profile_validation"), "profile_validation", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.contextCollisions) != 1 {
		t.Fatalf("expected one context collision: %#v", result.contextCollisions)
	}
	want := []string{fingerprintID("'shared"), fingerprintID("shared")}
	slices.Sort(want)
	if !slices.Equal(want, result.contextCollisions[0].RawContextFingerprints) {
		t.Fatalf("wrong raw contexts associated with emitted collision: got %v, want %v",
			result.contextCollisions[0].RawContextFingerprints, want)
	}
}

func TestValidateProfileAllowsIntentionalRawContextReuse(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - id: one
      title: One
      context: shared
      units: values
      dimensions:
        - selector: app_one
          name: one
    - id: two
      title: Two
      context: shared
      units: values
      dimensions:
        - selector: app_two
          name: two
`
	result := runValidation(t, profile, twoValueGaugesDump, "")
	if result.exitCode != 0 {
		t.Fatalf("intentional raw context reuse is not a wire-normalization collision\nreport:\n%s", result.stdout)
	}
	if len(result.report.ContextCollisions) != 0 {
		t.Fatalf("same raw context must not be reported as a normalization collision: %#v", result.report.ContextCollisions)
	}
}

func TestValidateProfileFindsEmptyChartWireID(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_value]
  charts:
    - id: "'"
      title: Value
      context: value
      units: values
      dimensions:
        - selector: app_value
          name: value
`
	result := runValidation(t, profile, "# TYPE app_value gauge\napp_value 1\n", "")
	requireFinding(t, result, "chart_wire_id_empty")
}

func TestInspectEmittedPlanFindsEmptyContextWireValue(t *testing.T) {
	action := chartengine.CreateChartAction{ChartID: "value"}
	action.Meta.Context = "'"
	result, err := inspectEmittedPlan(
		chartengine.Plan{Actions: []chartengine.EngineAction{action}},
		collectorJobFullName("profile_validation"),
		"profile_validation",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.emptyContexts) != 1 {
		t.Fatalf("expected one empty emitted context: %#v", result)
	}
}

func TestInspectEmittedPlanHandlesLargeChartLine(t *testing.T) {
	action := chartengine.CreateChartAction{ChartID: "value"}
	action.Meta.Title = strings.Repeat("x", 70*1024)
	result, err := inspectEmittedPlan(
		chartengine.Plan{Actions: []chartengine.EngineAction{action}},
		collectorJobFullName("profile_validation"),
		"profile_validation",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.emittedCharts != 1 {
		t.Fatalf("expected one emitted chart: %#v", result)
	}
}

func TestValidateProfileDoesNotLeakChartIDFromEmitterError(t *testing.T) {
	const sentinel = "sensitive-instance-value-"
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "  context_namespace: app\n", "")
	labelValue := sentinel + strings.Repeat("x", 1300)
	dump := fmt.Sprintf("# TYPE app_value gauge\napp_value{instance=%q} 1\n", labelValue)
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "chart_emit")
	if strings.Contains(result.stdout, sentinel) || strings.Contains(result.stderr, sentinel) {
		t.Fatalf("emitter failure leaked a label-derived chart ID\nstderr:\n%s\nreport:\n%s", result.stderr, result.stdout)
	}
}

func TestValidateProfileFindsLifecycleDimensionMaterializationLoss(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      lifecycle:
        dimensions:
          max_dims: 2
      dimensions:
        - selector: app_value
          name_from_label: state
`
	dump := "# TYPE app_value gauge\napp_value{state=\"a\"} 1\napp_value{state=\"b\"} 2\napp_value{state=\"c\"} 3\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 3 ||
		result.report.DimensionLosses[0].PlannedDimensions != 2 {
		t.Fatalf("unexpected lifecycle dimension loss report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsLifecycleLossAcrossSiblingDimensions(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_a, app_b, app_c]
  charts:
    - title: Values
      context: values
      units: values
      lifecycle:
        dimensions:
          max_dims: 2
      dimensions:
        - selector: app_a
          name: a
        - selector: app_b
          name: b
        - selector: app_c
          name: c
`
	dump := "# TYPE app_a gauge\napp_a 1\n# TYPE app_b gauge\napp_b 2\n# TYPE app_c gauge\napp_c 3\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 3 ||
		result.report.DimensionLosses[0].PlannedDimensions != 2 {
		t.Fatalf("unexpected sibling dimension loss report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsObservedSiblingDimensionNameCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  metrics: [app_one, app_two]
  charts:
    - title: Values
      context: values
      units: values
      dimensions:
        - selector: app_one
          name_from_label: state
        - selector: app_two
          name: x
`
	dump := "# TYPE app_one gauge\napp_one{state=\"x\"} 1\n# TYPE app_two gauge\napp_two 2\n"
	result := runValidation(t, profile, dump, "")
	requireFinding(t, result, "dimension_materialization_loss")
	if len(result.report.DimensionLosses) != 1 ||
		result.report.DimensionLosses[0].ObservedDimensions != 2 ||
		result.report.DimensionLosses[0].PlannedDimensions != 1 {
		t.Fatalf("unexpected sibling dimension collision report: %#v", result.report.DimensionLosses)
	}
}

func TestValidateProfileFindsRenderedIDCollision(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_one, app_two]
  charts:
    - title: One
      context: shared
      units: values
      dimensions:
        - selector: app_one
          name: one
    - title: Two
      context: shared
      units: values
      dimensions:
        - selector: app_two
          name: two
`
	result := runValidation(t, profile, twoValueGaugesDump, "")
	requireFinding(t, result, "rendered_id_collision")
	if len(result.report.Collisions) != 1 {
		t.Fatalf("collisions: got %#v", result.report.Collisions)
	}
	if result.report.Collisions[0].RenderedIDFingerprint == "" {
		t.Fatalf("collision ID fingerprint is empty: %#v", result.report.Collisions[0])
	}
}

func TestPlanRouteSummaryCountsDistinctSeries(t *testing.T) {
	summary := newPlanRouteSummary()
	summary.observe(chartengine.PlanRouteDiagnostic{
		Decision:       chartengine.PlanRouteAccepted,
		SeriesIdentity: metrix.SeriesIdentity{ID: "curated"},
	})
	summary.observe(chartengine.PlanRouteDiagnostic{
		Decision:       chartengine.PlanRouteAccepted,
		SeriesIdentity: metrix.SeriesIdentity{ID: "curated"},
	})
	summary.observe(chartengine.PlanRouteDiagnostic{
		Decision:       chartengine.PlanRouteAccepted,
		SeriesIdentity: metrix.SeriesIdentity{ID: "autogen"},
		Autogen:        true,
	})
	summary.observe(chartengine.PlanRouteDiagnostic{
		Decision:       chartengine.PlanRouteUnmatched,
		SeriesIdentity: metrix.SeriesIdentity{ID: "unmatched"},
	})

	scanned, autogen, unmatched := summary.counts()
	if scanned != 3 || autogen != 1 || unmatched != 1 {
		t.Fatalf("unexpected route counts: scanned=%d autogen=%d unmatched=%d", scanned, autogen, unmatched)
	}
}
