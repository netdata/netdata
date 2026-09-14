// SPDX-License-Identifier: GPL-3.0-or-later

package promvalidation

import (
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
)

func TestOptionalInstanceLabelsParticipateInValidationPolicy(t *testing.T) {
	instances := &charttpl.Instances{OptionalByLabels: []string{"pid"}}
	identity, err := chartIdentityLabels(instances)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := identity["pid"]; !ok {
		t.Fatalf("optional identity label missing from hierarchy policy: %#v", identity)
	}
	if _, ok := identity[globalChartIdentity]; ok {
		t.Fatalf("optional-only instance policy was misclassified as global: %#v", identity)
	}

	handled, wildcard, err := handledChartLabels(charttpl.Chart{Instances: instances})
	if err != nil {
		t.Fatal(err)
	}
	if wildcard {
		t.Fatal("optional identity label was misclassified as wildcard identity")
	}
	if _, ok := handled["pid"]; !ok {
		t.Fatalf("optional identity label was misclassified as aggregated: %#v", handled)
	}
}

func TestValidateProfileWarnsAboutObservedLabelsWithoutAnAuthoredRole(t *testing.T) {
	profile := replaceOnce(t, singleInstanceValueGaugeProfile, "selector: app_value", `selector: 'app_value{mode="sync"}'`)
	dump := `
# TYPE app_value gauge
app_value{instance="node-a",mode="sync",engine="sensitive-engine-value"} 1
`
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("label-role review must preserve author judgment\nreport:\n%s", result.stdout)
	}

	var message string
	for _, item := range result.report.Findings {
		if item.Code == "observed_label_aggregation" && item.Severity == "warning" {
			message = item.Message
			break
		}
	}
	if message == "" {
		t.Fatalf("missing observed-label aggregation prompt: %#v", result.report.Findings)
	}
	if !strings.Contains(message, "engine") {
		t.Fatalf("unaccounted label key is absent from warning: %q", message)
	}
	if strings.Contains(message, "instance") || strings.Contains(message, "mode") {
		t.Fatalf("identity and selector-routing labels were misreported as aggregated: %q", message)
	}
	if strings.Contains(result.stdout, "sensitive-engine-value") {
		t.Fatalf("label-role review leaked an observed label value:\n%s", result.stdout)
	}
}

func TestValidateProfileTreatsDistributionLabelNamesAsOrdinaryOnGaugeSeries(t *testing.T) {
	dump := `
# TYPE app_value gauge
app_value{instance="node-a",le="ordinary-gauge-label"} 1
`
	result := runValidation(t, singleInstanceValueGaugeProfile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("label-role review must preserve author judgment\nreport:\n%s", result.stdout)
	}

	var message string
	for _, item := range result.report.Findings {
		if item.Code == "observed_label_aggregation" && item.Severity == "warning" {
			message = item.Message
			break
		}
	}
	if !strings.Contains(message, "le") {
		t.Fatalf("an ordinary gauge label named le was mistaken for histogram structure: %q", message)
	}
}

func TestObservedLabelAggregationScansWriterSeriesOnce(t *testing.T) {
	templateYAML := `
version: v1
groups:
  - family: Example
    metrics: [app.one, app.two]
    charts:
      - title: One
        context: app.one
        units: values
        dimensions:
          - selector: app.one
            name: one
      - title: Two
        context: app.two
        units: values
        dimensions:
          - selector: app.two
            name: two
`
	spec, err := charttpl.DecodeYAML([]byte(templateYAML))
	if err != nil {
		t.Fatal(err)
	}
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		t.Fatal("collector store does not expose cycle control")
	}
	cycle := managed.CycleController()
	cycle.BeginCycle()
	meter := store.Write().SnapshotMeter("app")
	meter.Gauge("one").Observe(1, meter.LabelSet(metrix.Label{Key: "extra", Value: "a"}))
	meter.Gauge("two").Observe(2, meter.LabelSet(metrix.Label{Key: "extra", Value: "b"}))
	if err := cycle.CommitCycleSuccess(); err != nil {
		t.Fatal(err)
	}

	reader := store.Read(metrix.ReadRaw(), metrix.ReadFlatten())
	planned, err := prepareRoutePlan(reader, templateYAML, "prometheus_test")
	if err != nil {
		t.Fatal(err)
	}
	counting := &countingSeriesIdentityReader{Reader: reader}
	var got Report
	if err := addObservedLabelAggregationHeuristics(spec, enumerateChartRefs(spec), counting, planned.routes, &got); err != nil {
		t.Fatal(err)
	}
	if counting.passes != 1 {
		t.Fatalf("writer series scanned %d times, want one pass regardless of chart count", counting.passes)
	}
}

type countingSeriesIdentityReader struct {
	metrix.Reader
	passes int
}

func (r *countingSeriesIdentityReader) ForEachSeriesIdentity(
	fn func(metrix.SeriesIdentity, metrix.SeriesMeta, string, metrix.LabelView, metrix.SampleValue),
) {
	r.passes++
	r.Reader.ForEachSeriesIdentity(fn)
}

func TestValidateProfileWarnsOnSiblingIdentityMismatchWithoutReplacingJudgment(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: Per Node
      metrics: [app_node_value]
      charts:
        - title: Node Value
          context: node_value
          units: values
          instances:
            by_labels: [node]
          dimensions:
            - selector: app_node_value
              name: value
    - family: Global
      metrics: [app_global_value]
      charts:
        - title: Global Value
          context: global_value
          units: values
          dimensions:
            - selector: app_global_value
              name: value
`
	dump := "# TYPE app_node_value gauge\napp_node_value{node=\"a\"} 1\n# TYPE app_global_value gauge\napp_global_value 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("heuristic must remain a warning, not a false mechanical failure\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "sibling_identity_mismatch", "warning") {
		t.Fatalf("missing sibling identity review prompt: %#v", result.report.Findings)
	}
}

func TestValidateProfileTreatsGlobalSiblingChartsAsCommonIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: First
      metrics: [app_one]
      charts:
        - title: One
          context: one
          units: values
          dimensions:
            - selector: app_one
              name: one
    - family: Second
      metrics: [app_two]
      charts:
        - title: Two
          context: two
          units: values
          dimensions:
            - selector: app_two
              name: two
`
	result := runValidation(t, profile, twoValueGaugesDump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	if hasFinding(result.report, "sibling_identity_mismatch", "warning") {
		t.Fatalf("global siblings share the same global identity: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenDisplayedFamilyMixesEntityIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  metrics: [app_global_value, app_server_value]
  charts:
    - title: Global Value
      context: global_value
      units: values
      dimensions:
        - selector: app_global_value
          name: value
    - title: Server Value
      context: server_value
      units: values
      instances:
        by_labels: [server]
      dimensions:
        - selector: app_server_value
          name: value
`
	dump := "# TYPE app_global_value gauge\napp_global_value 1\n# TYPE app_server_value gauge\napp_server_value{server=\"a\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("mixed family identity must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "family_identity_mixed", "warning") {
		t.Fatalf("missing mixed displayed-family identity warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenChildDropsDeclaredParentIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_server_value, app_database_value]
  charts:
    - title: Server Value
      family: Servers
      context: server_value
      units: values
      dimensions:
        - selector: app_server_value
          name: value
    - title: Database Value
      family: Databases
      context: database_value
      units: values
      instances:
        by_labels: [database]
      dimensions:
        - selector: app_database_value
          name: value
`
	dump := "# TYPE app_server_value gauge\napp_server_value{server=\"sensitive-server\"} 1\n# TYPE app_database_value gauge\napp_database_value{server=\"sensitive-server\",database=\"sensitive-database\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("parent identity loss must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "identity_parent_labels_dropped", "warning") {
		t.Fatalf("missing parent identity loss warning: %#v", result.report.Findings)
	}
	if strings.Contains(result.stdout, "sensitive-") {
		t.Fatalf("identity hierarchy warning leaked an observed label value:\n%s", result.stdout)
	}
}

func TestValidateProfileWarnsWhenWildcardChildExcludesParentIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_database_value]
  charts:
    - title: Database Value
      context: database_value
      units: values
      instances:
        by_labels: ['*', '!server']
      dimensions:
        - selector: app_database_value
          name: value
`
	dump := "# TYPE app_database_value gauge\napp_database_value{server=\"a\",database=\"main\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("wildcard parent identity loss must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "identity_parent_labels_dropped", "warning") {
		t.Fatalf("missing wildcard parent identity loss warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileWarnsWhenGroupOverrideDropsParentIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  groups:
    - family: Databases
      chart_defaults:
        instances:
          by_labels: [database]
      metrics: [app_database_value]
      charts:
        - title: Database Value
          context: database_value
          units: values
          dimensions:
            - selector: app_database_value
              name: value
`
	dump := "# TYPE app_database_value gauge\napp_database_value{server=\"a\",database=\"main\"} 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("group parent identity loss must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "identity_parent_labels_dropped", "warning") {
		t.Fatalf("missing group parent identity loss warning: %#v", result.report.Findings)
	}
}

func TestValidateProfileAcceptsMonotonicNestedEntityIdentity(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  chart_defaults:
    instances:
      by_labels: [server]
  metrics: [app_server_value]
  charts:
    - title: Server Value
      context: server_value
      units: values
      dimensions:
        - selector: app_server_value
          name: value
  groups:
    - family: Databases
      chart_defaults:
        instances:
          by_labels: [server, database]
      metrics: [app_database_value]
      charts:
        - title: Database Value
          context: database_value
          units: values
          dimensions:
            - selector: app_database_value
              name: value
      groups:
        - family: Tables
          chart_defaults:
            instances:
              by_labels: [server, database, table]
          metrics: [app_table_value]
          charts:
            - title: Table Value
              context: table_value
              units: values
              dimensions:
                - selector: app_table_value
                  name: value
`
	dump := "# TYPE app_server_value gauge\napp_server_value{server=\"a\"} 1\n# TYPE app_database_value gauge\napp_database_value{server=\"a\",database=\"main\"} 2\n# TYPE app_table_value gauge\napp_table_value{server=\"a\",database=\"main\",table=\"orders\"} 3\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("expected PASS\nreport:\n%s", result.stdout)
	}
	for _, code := range []string{
		"family_identity_mixed",
		"identity_parent_labels_dropped",
		"sibling_identity_mismatch",
	} {
		if hasFinding(result.report, code, "warning") {
			t.Fatalf("unexpected %s warning: %#v", code, result.report.Findings)
		}
	}
}

func TestValidateProfileWarnsWhenSiblingFamilyPathIsRepeated(t *testing.T) {
	profile := `
match: app_*
app: app
template:
  family: Example
  context_namespace: app
  groups:
    - family: Storage
      metrics: [app_reads]
      charts:
        - title: Reads
          context: reads
          units: operations/s
          algorithm: incremental
          dimensions:
            - selector: app_reads
              name: reads
    - family: Storage
      metrics: [app_writes]
      charts:
        - title: Writes
          context: writes
          units: operations/s
          algorithm: incremental
          dimensions:
            - selector: app_writes
              name: writes
`
	dump := "# TYPE app_reads counter\napp_reads 1\n# TYPE app_writes counter\napp_writes 2\n"
	result := runValidation(t, profile, dump, "")
	if result.exitCode != 0 {
		t.Fatalf("duplicate sibling family must remain an advisory\nreport:\n%s", result.stdout)
	}
	if !hasFinding(result.report, "duplicate_sibling_family", "warning") {
		t.Fatalf("missing duplicate sibling family warning: %#v", result.report.Findings)
	}
}
