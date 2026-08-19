// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"context"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestValidateSemanticSnapshotUsesProductionFacts(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	profile := `
match: app_*
app: app
fallback_type:
  gauge: [app_value]
relabeling:
  - match: app_value
    metric_relabel_configs:
      - source_labels: [node]
        target_label: instance
        regex: (.*)
        replacement: $1
        action: replace
template:
  family: Example
  context_namespace: app
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      label_promotion: []
      instances:
        by_labels: [instance]
      dimensions:
        - selector: app_value
          name: value
`
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, []byte("app_value{node=\"node-a\"} 7\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	report := validateTestMode(context.Background(), Options{
		ProfilePath: profilePath,
		DumpPath:    dumpPath,
	}, validationMode{semanticFacts: true})
	snapshot := report.ResultSnapshot().Semantics
	if snapshot == nil {
		t.Fatalf("semantic snapshot is nil; findings=%#v", report.Findings)
	}
	if snapshot.ContextRoot != "prometheus.app" || !slices.Equal(snapshot.SelectedProfiles, []string{"candidate"}) {
		t.Fatalf("unexpected snapshot header: %#v", snapshot)
	}
	if len(snapshot.Profiles) != 1 || snapshot.Profiles[0].ContextNamespace != "app" ||
		len(snapshot.Profiles[0].FallbackRules) != 1 ||
		snapshot.Profiles[0].FallbackRules[0].RuntimePath != "fallback_type.gauge[0]" {
		t.Fatalf("unexpected static profile facts: %#v", snapshot.Profiles)
	}
	if len(snapshot.Sources) != 1 {
		t.Fatalf("source count: got %d, want 1", len(snapshot.Sources))
	}
	source := snapshot.Sources[0]
	if source.Value != 7 || source.FinalMetricName != "app_value" || len(source.RelabelRules) != 1 ||
		source.RelabelRules[0].RuntimePath != "relabeling[0].metric_relabel_configs[0]" {
		t.Fatalf("unexpected relabel facts: %#v", source)
	}
	if len(source.Routes) != 1 {
		t.Fatalf("route count: got %d, want 1; source=%#v", len(source.Routes), source)
	}
	route := source.Routes[0]
	if route.Profile != "candidate" || route.TemplatePath != "template.charts[0]" ||
		route.Context != "prometheus.app.value" || route.DisplayedFamily != "Example" ||
		!slices.Equal(route.IdentityLabels, []string{"instance"}) ||
		!slices.Equal(route.ChartLabels, []string{"instance"}) ||
		!slices.Equal(route.ChartLabelValues, []promreplay.SemanticLabel{{Name: "instance", Value: "node-a"}}) ||
		route.PromotionMode != "identity_only" || route.Algorithm != "absolute" ||
		route.SeriesKind != "gauge" || route.Aggregation != "sum" ||
		route.Units != "values" || route.Multiplier != 1 || route.Divisor != 1 ||
		route.Presentation != "line" || route.ContributorCount != 1 {
		t.Fatalf("unexpected effective route: %#v", route)
	}
	var createdChart, createdDimension, updatedDimension *promreplay.SemanticPlanAction
	for i := range snapshot.PlanActions {
		action := &snapshot.PlanActions[i]
		switch action.Kind {
		case "create_chart":
			createdChart = action
		case "create_dimension":
			createdDimension = action
		case "update_dimension":
			updatedDimension = action
		}
	}
	if createdChart == nil || createdChart.WireTypeID == "" || createdChart.WireChartID == "" ||
		!slices.Equal(createdChart.Labels, route.ChartLabelValues) {
		t.Fatalf("unexpected created chart action: %#v", createdChart)
	}
	if createdDimension == nil || createdDimension.WireDimensionID == "" ||
		createdDimension.DimensionName != route.DimensionName {
		t.Fatalf("unexpected created dimension action: %#v", createdDimension)
	}
	if updatedDimension == nil || !updatedDimension.Float || updatedDimension.Float64 != 7 ||
		updatedDimension.DimensionName != route.DimensionName {
		t.Fatalf("unexpected updated dimension action: %#v", updatedDimension)
	}

	// ResultSnapshot must not expose report-owned slices.
	snapshot.Sources[0].Routes[0].IdentityLabels[0] = "mutated"
	snapshot.Sources[0].Routes[0].ChartLabelValues[0].Value = "mutated"
	snapshot.PlanActions[0].Labels = append(snapshot.PlanActions[0].Labels, promreplay.SemanticLabel{Name: "mutated"})
	if got := report.ResultSnapshot().Semantics.Sources[0].Routes[0].IdentityLabels[0]; got != "instance" {
		t.Fatalf("ResultSnapshot aliases report state: got %q", got)
	}
	if got := report.ResultSnapshot().Semantics.Sources[0].Routes[0].ChartLabelValues[0].Value; got != "node-a" {
		t.Fatalf("ResultSnapshot aliases route label state: got %q", got)
	}
}

func TestValidateSemanticSnapshotOwnsChartsInProductionMergeOrder(t *testing.T) {
	dir := t.TempDir()
	zetaPath := filepath.Join(dir, "zeta.yaml")
	alphaPath := filepath.Join(dir, "alpha.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	profile := func(match, namespace string) string {
		return `
match: ` + match + `
app: app
template:
  family: ` + namespace + `
  context_namespace: ` + namespace + `
  metrics: [` + match + `]
  charts:
    - title: Value
      context: value
      units: values
      dimensions:
        - selector: ` + match + `
          name: value
`
	}
	if err := os.WriteFile(zetaPath, []byte(profile("zeta_value", "zeta")), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(alphaPath, []byte(profile("alpha_value", "alpha")), 0o600); err != nil {
		t.Fatal(err)
	}
	dump := `
# TYPE alpha_value gauge
alpha_value 1
# TYPE zeta_value gauge
zeta_value 2
`
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}

	report := validateTestMode(context.Background(), Options{
		ProfilePath:            zetaPath,
		SupportingProfilePaths: []string{alphaPath},
		DumpPath:               dumpPath,
	}, validationMode{semanticFacts: true, automaticProfileSelection: true})
	snapshot := report.ResultSnapshot().Semantics
	if snapshot == nil {
		t.Fatalf("semantic snapshot is nil; findings=%#v", report.Findings)
	}
	if want := []string{"alpha", "zeta"}; !slices.Equal(report.Profiles.Selected, want) {
		t.Fatalf("selected profile order = %v, want production catalog order %v", report.Profiles.Selected, want)
	}
	if want := []string{"alpha", "zeta"}; !slices.Equal(snapshot.SelectedProfiles, want) {
		t.Fatalf("semantic selected profiles = %v, want %v", snapshot.SelectedProfiles, want)
	}
	owners := make(map[string]string)
	for _, source := range snapshot.Sources {
		if len(source.Routes) != 1 {
			t.Fatalf("source %q routes = %#v, want one", source.MetricName, source.Routes)
		}
		owners[source.MetricName] = source.Routes[0].Profile
	}
	if want := map[string]string{"alpha_value": "alpha", "zeta_value": "zeta"}; !maps.Equal(owners, want) {
		t.Fatalf("route owners = %v, want %v", owners, want)
	}
}

func TestValidateSemanticSnapshotKeepsTypedFamilyRelabelTracesPerPhysicalSample(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	profile := `
match: app_*
relabeling:
  - match: app_latency_seconds_bucket app_latency_seconds_sum app_latency_seconds_count
    metric_relabel_configs:
      - action: drop
template:
  family: Example
  context_namespace: app
  metrics: [app_value]
  charts:
    - title: Value
      context: value
      units: values
      dimensions:
        - selector: app_value
          name: value
`
	dump := `
# TYPE app_value gauge
app_value 1
# TYPE app_latency_seconds histogram
app_latency_seconds_bucket{le="1"} 1
app_latency_seconds_bucket{le="+Inf"} 2
app_latency_seconds_sum 1.5
app_latency_seconds_count 2
`
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}

	report := validateTestMode(context.Background(), Options{
		ProfilePath: profilePath,
		DumpPath:    dumpPath,
	}, validationMode{semanticFacts: true})
	snapshot := report.ResultSnapshot().Semantics
	if snapshot == nil {
		t.Fatalf("semantic snapshot is nil; findings=%#v", report.Findings)
	}
	want := map[string]int{
		"app_latency_seconds_bucket": 2,
		"app_latency_seconds_sum":    1,
		"app_latency_seconds_count":  1,
	}
	for _, source := range snapshot.Sources {
		if _, ok := want[source.MetricName]; !ok {
			continue
		}
		if len(source.RelabelRules) != 1 || source.RelabelRules[0].InputMetricName != source.MetricName {
			t.Fatalf("source %q has another physical sample's relabel trace: %#v", source.MetricName, source.RelabelRules)
		}
		want[source.MetricName]--
	}
	for name, remaining := range want {
		if remaining != 0 {
			t.Fatalf("source %q remaining occurrences = %d, want 0", name, remaining)
		}
	}
}

func TestValidateDoesNotBuildSemanticSnapshotByDefault(t *testing.T) {
	result := runValidation(t, singleInstanceValueGaugeProfile, "# TYPE app_value gauge\napp_value{instance=\"one\"} 1\n", "")
	if result.report.ResultSnapshot().Semantics != nil {
		t.Fatal("ordinary validation unexpectedly built semantic facts")
	}
}

func TestValidateSemanticSnapshotIdentifiesProfileSuppressedWriterSeries(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	profile := `
match: app_*
app: app
autogen:
  selector:
    deny: [app_value]
template:
  family: Example
  context_namespace: app
  metrics: [app_value, app_other]
  charts:
    - title: Other
      context: other
      units: values
      dimensions:
        - selector: app_other
          name: other
`
	dump := "# TYPE app_value gauge\napp_value 1\n# TYPE app_other gauge\napp_other 2\n"
	if err := os.WriteFile(profilePath, []byte(profile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}
	report := validateTestMode(context.Background(), Options{
		ProfilePath: profilePath, DumpPath: dumpPath,
	}, validationMode{semanticFacts: true})
	if !report.Passed() || !hasFinding(report, "profile_suppressed_series", "warning") {
		t.Fatalf("raw validator must retain the profile-suppression warning: %#v", report.Findings)
	}
	snapshot := report.ResultSnapshot().Semantics
	if snapshot == nil {
		t.Fatalf("semantic snapshot is nil: %#v", report.Findings)
	}
	if len(snapshot.Profiles) != 1 ||
		!slices.Equal(snapshot.Profiles[0].AutogenSelectorDeny, []string{"app_value"}) ||
		len(snapshot.Profiles[0].AutogenSelectorAllow) != 0 {
		t.Fatalf("unexpected profile selector facts: %#v", snapshot.Profiles)
	}
	byName := make(map[string]struct {
		writer, autogen, unmatched, suppressed, routes int
	}, len(snapshot.Sources))
	for _, source := range snapshot.Sources {
		byName[source.MetricName] = struct {
			writer, autogen, unmatched, suppressed, routes int
		}{source.WriterSeries, source.AutogenSeries, source.UnmatchedSeries, len(source.AutogenSuppressions), len(source.Routes)}
		if source.MetricName == "app_value" {
			want := []promreplay.SemanticAutogenSuppression{{Profile: "candidate", Family: "app_value"}}
			if !slices.Equal(source.AutogenSuppressions, want) {
				t.Fatalf("app_value suppressions = %#v, want %#v", source.AutogenSuppressions, want)
			}
		}
	}
	if got := byName["app_value"]; got != (struct {
		writer, autogen, unmatched, suppressed, routes int
	}{1, 0, 0, 1, 0}) {
		t.Fatalf("app_value facts = %#v", got)
	}
	if got := byName["app_other"]; got != (struct {
		writer, autogen, unmatched, suppressed, routes int
	}{1, 0, 0, 0, 1}) {
		t.Fatalf("app_other facts = %#v", got)
	}
	for index := range snapshot.Sources {
		if snapshot.Sources[index].MetricName == "app_value" {
			snapshot.Sources[index].AutogenSuppressions[0].Profile = "mutated"
		}
	}
	for _, source := range report.ResultSnapshot().Semantics.Sources {
		if source.MetricName == "app_value" && source.AutogenSuppressions[0].Profile != "candidate" {
			t.Fatalf("ResultSnapshot aliases suppression state: %#v", source.AutogenSuppressions)
		}
	}
}

func TestValidateSemanticSnapshotLinksEveryDistributionComponent(t *testing.T) {
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "candidate.yaml")
	dumpPath := filepath.Join(dir, "metrics.prom")
	if err := os.WriteFile(profilePath, []byte(validProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	dump := strings.Replace(validDump, `le="0.5"`, `le="1.0"`, 1)
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}
	report := validateTestMode(context.Background(), Options{
		ProfilePath: profilePath, DumpPath: dumpPath,
	}, validationMode{semanticFacts: true})
	snapshot := report.ResultSnapshot().Semantics
	if snapshot == nil {
		t.Fatalf("semantic snapshot is nil; findings=%#v", report.Findings)
	}
	if len(snapshot.Sources) != 10 {
		t.Fatalf("source count: got %d, want 10", len(snapshot.Sources))
	}
	components := make(map[string]int)
	for _, source := range snapshot.Sources {
		components[source.Component]++
		if source.Terminal != nil || len(source.Routes) != 1 {
			t.Fatalf("source component was not linked exactly once: %#v", source)
		}
		if got := source.Routes[0].MetricName; got != source.MetricName {
			t.Fatalf("source %s/%s linked to route metric %q", source.MetricName, source.Component, got)
		}
	}
	want := map[string]int{
		"scalar": 2, "histogram_bucket": 2, "histogram_sum": 1, "histogram_count": 1,
		"summary_quantile": 2, "summary_sum": 1, "summary_count": 1,
	}
	if !maps.Equal(components, want) {
		t.Fatalf("component counts: got %v, want %v", components, want)
	}
}
