// SPDX-License-Identifier: GPL-3.0-or-later

package promproof

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/input"
	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestVerifyCompiledCatalogReconcilesPersistentObservations(t *testing.T) {
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	proofDirectory := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app")
	externalDirectory := filepath.Join(testdataRoot, "prometheus", "profiles", "app")
	mustWriteFile(t, filepath.Join(proofDirectory, DescriptorFilename), replayDescriptor)
	mustWriteFile(t, filepath.Join(proofDirectory, "PROFILE-DESIGN.yaml"), replayProfileDesign)
	mustWriteFile(t, filepath.Join(proofDirectory, "OPERATOR-MODEL.md"), "# Operator model\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(StockProfileRoot), "app.yaml"), "match: app_*\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "SOURCE-SEMANTICS.yaml"), replaySourceSemantics)
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "present.prom"), "present\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "absent.prom"), "absent\n")

	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, bundles)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = VerifyCompiledCatalog(
		context.Background(),
		repoRoot,
		testdataRoot,
		catalog,
		func(_ context.Context, input prominput.ReplayCase) ([]promreplay.Result, error) {
			called = true
			if input.DefaultJobName != "app" || len(input.FixturePaths) != 2 ||
				len(input.SupportingProfilePaths) != 0 {
				t.Fatalf("unexpected replay input: %#v", input)
			}
			first := replaySemanticSnapshot()
			second := promreplay.CloneSemanticSnapshot(first)
			second.Sources = nil
			second.PlanActions = []promreplay.SemanticPlanAction{{
				Kind: "update_dimension", ChartID: "requests", DimensionName: "total", IsEmpty: true, Float: true,
			}}
			return []promreplay.Result{
				{Snapshot: promreplay.Snapshot{
					Semantics: first,
				}},
				{Snapshot: promreplay.Snapshot{Semantics: second}},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("replay callback was not called")
	}
}

func TestVerifyReplayResultRequiresDeclaredFailureCode(t *testing.T) {
	err := verifyReplayResult(
		ExpectedResult{Verdict: "FAIL", Findings: []string{"source_mismatch"}},
		promreplay.Result{Snapshot: promreplay.Snapshot{Errors: map[string]int{"other": 1}}},
	)
	if err == nil {
		t.Fatal("verifyReplayResult() succeeded without the required objective finding")
	}
}

func TestCloneReplayResultCopiesOnlyMutableFindings(t *testing.T) {
	semantics := replaySemanticSnapshot()
	input := promreplay.Result{Snapshot: promreplay.Snapshot{
		Errors:                       map[string]int{"original": 1},
		UnsupportedFindingSeverities: map[string]int{"future": 1},
		Semantics:                    semantics,
	}}

	cloned := cloneReplayResult(input)
	cloned.Errors["added"] = 1
	if input.Errors["added"] != 0 {
		t.Fatalf("cloned errors alias input: %v", input.Errors)
	}
	if cloned.Semantics != semantics {
		t.Fatal("clone duplicated the read-only semantic snapshot")
	}
}

func TestVerifyCompiledCatalogAcceptsDeclaredValidatorFailureWithoutSemantics(t *testing.T) {
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	proofDirectory := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app")
	externalDirectory := filepath.Join(testdataRoot, "prometheus", "profiles", "app")
	descriptor := `version: v1
profile: app
cases:
  invalid_source:
    environment:
      app: {}
    fixture: fixtures/invalid.prom
    coverage: false
    expected:
      verdict: FAIL
      findings: [duplicate_source_sample]
  valid_source:
    environment:
      app: {}
    fixture: fixtures/valid.prom
    coverage: true
    expected: {verdict: PASS}
`
	mustWriteFile(t, filepath.Join(proofDirectory, DescriptorFilename), descriptor)
	mustWriteFile(t, filepath.Join(proofDirectory, "PROFILE-DESIGN.yaml"), replayProfileDesign)
	mustWriteFile(t, filepath.Join(proofDirectory, "OPERATOR-MODEL.md"), "# Operator model\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(StockProfileRoot), "app.yaml"), "match: app_*\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "SOURCE-SEMANTICS.yaml"), replaySourceSemantics)
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "invalid.prom"), "invalid\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "valid.prom"), "valid\n")

	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, bundles)
	if err != nil {
		t.Fatal(err)
	}
	err = VerifyCompiledCatalog(
		context.Background(),
		repoRoot,
		testdataRoot,
		catalog,
		func(_ context.Context, input prominput.ReplayCase) ([]promreplay.Result, error) {
			if filepath.Base(input.FixturePaths[0]) == "valid.prom" {
				return []promreplay.Result{{Snapshot: promreplay.Snapshot{
					Semantics: replaySemanticSnapshot(),
				}}}, nil
			}
			return []promreplay.Result{{Snapshot: promreplay.Snapshot{
				Errors: map[string]int{"duplicate_source_sample": 1},
			}}}, nil
		},
	)
	if err != nil {
		t.Fatalf("VerifyCompiledCatalog() rejected declared validator failure: %v", err)
	}
}

func TestVerifyCompiledCatalogAcceptsDeclaredSemanticFailure(t *testing.T) {
	repoRoot := t.TempDir()
	testdataRoot := t.TempDir()
	proofDirectory := filepath.Join(repoRoot, filepath.FromSlash(ProofRoot), "app")
	externalDirectory := filepath.Join(testdataRoot, "prometheus", "profiles", "app")
	descriptor := `version: v1
profile: app
cases:
  invalid_source:
    environment:
      app: {}
    fixture: fixtures/invalid.prom
    coverage: false
    expected:
      verdict: FAIL
      findings: [semantic_source_mismatch]
  valid_source:
    environment:
      app: {}
    fixture: fixtures/valid.prom
    coverage: true
    expected: {verdict: PASS}
`
	mustWriteFile(t, filepath.Join(proofDirectory, DescriptorFilename), descriptor)
	mustWriteFile(t, filepath.Join(proofDirectory, "PROFILE-DESIGN.yaml"), replayProfileDesign)
	mustWriteFile(t, filepath.Join(proofDirectory, "OPERATOR-MODEL.md"), "# Operator model\n")
	mustWriteFile(t, filepath.Join(repoRoot, filepath.FromSlash(StockProfileRoot), "app.yaml"), "match: app_*\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "SOURCE-SEMANTICS.yaml"), replaySourceSemantics)
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "invalid.prom"), "invalid\n")
	mustWriteFile(t, filepath.Join(externalDirectory, "fixtures", "valid.prom"), "valid\n")

	bundles, err := Discover(repoRoot)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := LoadCompiledCatalog(context.Background(), repoRoot, testdataRoot, bundles)
	if err != nil {
		t.Fatal(err)
	}
	err = VerifyCompiledCatalog(
		context.Background(),
		repoRoot,
		testdataRoot,
		catalog,
		func(_ context.Context, input prominput.ReplayCase) ([]promreplay.Result, error) {
			snapshot := replaySemanticSnapshot()
			if filepath.Base(input.FixturePaths[0]) == "invalid.prom" {
				snapshot.Sources[0].MetricName = "wrong_requests_total"
			}
			return []promreplay.Result{{Snapshot: promreplay.Snapshot{Semantics: snapshot}}}, nil
		},
	)
	if err != nil {
		t.Fatalf("VerifyCompiledCatalog() rejected declared semantic failure: %v", err)
	}
}

func replaySemanticSnapshot() *promreplay.SemanticSnapshot {
	return &promreplay.SemanticSnapshot{
		ContextRoot:      "prometheus.app",
		SelectedProfiles: []string{"app"},
		Profiles: []promreplay.SemanticProfile{{
			Name: "app", Match: "app_*", ContextNamespace: "app",
			Charts: []promreplay.SemanticChartPolicy{{
				RuntimePath: "template.charts[0]", TemplateID: "template-0",
				Dimensions: []promreplay.SemanticDimensionPolicy{{Index: 0}},
			}},
		}},
		Sources: []promreplay.SemanticSource{{
			OccurrenceID: "source-1", MetricName: "app_requests_total", Component: "scalar",
			PrometheusType: "counter", Value: 10, FinalMetricName: "app_requests_total", WriterSeries: 1,
			Routes: []promreplay.SemanticRoute{{
				Profile: "app", TemplatePath: "template.charts[0]", MetricName: "app_requests_total",
				ChartID: "requests", Context: "prometheus.app.requests", DisplayedFamily: "Traffic/Requests",
				DimensionIndex: 0, DimensionName: "total", PromotionMode: "automatic",
				Algorithm: "incremental", SeriesKind: "counter", Aggregation: "sum",
				Units: "requests/s", Multiplier: 1, Divisor: 1, Presentation: "line", ContributorCount: 1,
			}},
		}},
		PlanActions: []promreplay.SemanticPlanAction{
			{
				Kind: "create_chart", ChartTemplateID: "template-0", ChartID: "requests",
				Context: "prometheus.app.requests", DisplayedFamily: "Traffic/Requests",
				Units: "requests/s", Presentation: "line", WireTypeID: "prometheus.app",
				WireChartID: "requests", WireContext: "prometheus.app.requests",
			},
			{
				Kind: "create_dimension", ChartID: "requests", DimensionName: "total",
				Context: "prometheus.app.requests", DisplayedFamily: "Traffic/Requests",
				Units: "requests/s", Presentation: "line", Algorithm: "incremental", Float: true,
				Multiplier: 1, Divisor: 1, WireTypeID: "prometheus.app", WireChartID: "requests",
				WireDimensionID: "total",
			},
			{
				Kind: "update_dimension", ChartID: "requests", DimensionName: "total", Float: true, Float64: 10,
			},
		},
	}
}

const replayDescriptor = `version: v1
profile: app
cases:
  lifecycle:
    environment:
      app: {}
    coverage: true
    steps:
      - fixture: fixtures/present.prom
        expected: {verdict: PASS}
        observations:
          requests#total:
            state: current
            predicates: {membership: establish, aggregate: matches_reducer, identity: establish}
      - fixture: fixtures/absent.prom
        expected: {verdict: PASS}
        observations:
          requests#total:
            state: stale
            predicates: {membership: removed, aggregate: became_gap, identity: unchanged}
`

const replayProfileDesign = `version: v1
profile: app
match: app_*
namespace: app
composition: {supports: {}}
entities:
  service:
    grain: service
    identity: {required: [], optional: []}
label_policies: {}
reduction_policies: {}
normalizations: {}
exclusions: {}
limitations: {}
views:
  requests:
    family: Traffic/Requests
    question: How many requests complete?
    entity: service
    inputs:
      total:
        signal: requests
        components: [total]
    labels: {dimensions: {}, promote: [], omit: {}}
`

const replaySourceSemantics = `version: v1
profile: app
upstreams:
  exporter:
    repository: owner/exporter
    commit: 0123456789abcdef0123456789abcdef01234567
evidence:
  registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:10]
    claim: The source registers one request counter.
  population:
    kind: population
    upstream: exporter
    locations: [metrics.go:11]
    claim: One increment represents one completed request.
  lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:12]
    claim: The counter resets only with process state.
  unit:
    kind: unit
    upstream: exporter
    locations: [metrics.go:13]
    claim: The counter measures completed requests.
environment: {axes: {}, policies: {}}
component_policies: {}
label_policies: {}
signals:
  requests:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: app_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [registration]
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [population]
    components:
      total:
        wire_role: scalar
        lifecycle: {kind: cumulative, evidence: [lifecycle]}
        unit:
          quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed
          evidence: [unit]
    labels: {}
    functional_dependencies: {}
relationships: {}
state_encodings: {}
source_exclusions: {}
`
