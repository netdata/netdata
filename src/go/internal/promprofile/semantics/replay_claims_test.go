// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionClaimsValidatesStateEncoding(t *testing.T) {
	design := strings.Replace(validProfileDesignV1,
		"dimensions: {}", "dimensions: {status: {render: label_value}}", 1)
	program := compileTestSemanticContract(t, design, sourceWithStateEncodingV1(false))
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = stateSources()
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}
	if len(reconciled.Claims) != 1 || reconciled.Claims[0].Kind != "state_encoding" {
		t.Fatalf("claims = %#v, want state-encoding witness", reconciled.Claims)
	}

	snapshot.Sources = snapshot.Sources[:2]
	reconciled, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	err = semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), `missing state "invalid"`) {
		t.Fatalf("ReconcileProductionClaims() error = %v, want missing-state failure", err)
	}
}

func TestReconcileProductionClaimsValidatesEquivalentRelationship(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, sourceWithEquivalentLegacySignalV1(true))
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		semanticClaimSource("current", "example_requests_total", 7),
		semanticClaimSource("legacy", "example_legacy_requests_total", 7),
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}
	if len(reconciled.Claims) != 1 || reconciled.Claims[0].ID != "request_alias" {
		t.Fatalf("claims = %#v, want equivalent relationship witness", reconciled.Claims)
	}

	snapshot.Sources[1].Value = 8
	reconciled, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	err = semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), "equivalent members have values") {
		t.Fatalf("ReconcileProductionClaims() error = %v, want relationship-value failure", err)
	}
}

func TestReconcileProductionClaimsValidatesProjectedEquivalentRelationship(t *testing.T) {
	source := sourceWithEquivalentLegacySignalV1(true)
	source = strings.Replace(source, "  request_lifecycle:\n", `  instance_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:24]
    claim: The instance label identifies the exporter service.
  zone_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:25]
    claim: The zone label distinguishes the canonical representation.
  request_lifecycle:
`, 1)
	labels := `    labels:
      instance_id:
        meaning: Exporter service identity.
        presence: required
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: stable
        evidence: [instance_label]
      source_zone:
        meaning: Canonical source zone.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: bounded_configuration}
        stability: stable
        evidence: [zone_label]`
	source = strings.Replace(source, "    labels: {}", labels, 1)
	source = strings.Replace(source, "    labels: {}", labels, 1)
	source = strings.Replace(source,
		"    right: {signal: legacy_requests, components: [value]}\n",
		"    right: {signal: legacy_requests, components: [value]}\n    group_by: [instance_id]\n", 1)

	design := strings.Replace(validProfileDesignV1, `    identity:
      required: []
      optional: []`, `    identity:
      required: [instance_id]
      optional: [source_zone]`, 1)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		semanticClaimSourceWithLabels("current", "example_requests_total", 7, "instance_id", "service-a"),
		semanticClaimSourceWithLabels(
			"legacy", "example_legacy_requests_total", 7,
			"instance_id", "service-a", "source_zone", "zone-a",
		),
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}

	snapshot.Sources[1].Value = 8
	reconciled, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	err = semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), "equivalent members have values") {
		t.Fatalf("ReconcileProductionClaims() error = %v, want projected relationship-value failure", err)
	}

	invalid := strings.Replace(source, "group_by: [instance_id]", "group_by: [missing]", 1)
	contract := loadTestSemanticContract(t, design, invalid, "", "")
	_, err = CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `group label "missing" must exist on left and right signals`) {
		t.Fatalf("CompileSemanticContract() error = %v, want unknown projected-identity label failure", err)
	}
}

func TestReconcileProductionClaimsValidatesSumProjectionRelationship(t *testing.T) {
	design, source := sumProjectionContract(false)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		semanticClaimSourceWithLabels("fine-a-get", "example_requests_total", 2, "handler", "a", "method", "GET"),
		semanticClaimSourceWithLabels("fine-a-post", "example_requests_total", 3, "handler", "a", "method", "POST"),
		semanticClaimSourceWithLabels("fine-b-get", "example_requests_total", 4, "handler", "b", "method", "GET"),
		semanticClaimSourceWithLabels("coarse-a", "example_request_count_total", 5, "handler", "a"),
		semanticClaimSourceWithLabels("coarse-b", "example_request_count_total", 4, "handler", "b"),
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}
	if len(reconciled.Claims) != 1 || reconciled.Claims[0].ID != "request_count_projection" {
		t.Fatalf("claims = %#v, want sum-projection relationship witness", reconciled.Claims)
	}

	snapshot.Sources[3].Value = 6
	reconciled, err = semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	err = semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), `group "handler=a" coarse value 6 differs from fine sum 5`) {
		t.Fatalf("ReconcileProductionClaims() error = %v, want per-group projection failure", err)
	}
}

func TestReconcileProductionClaimsGroupsMissingOptionalProjectionLabels(t *testing.T) {
	design, source := sumProjectionContract(false)
	design = strings.Replace(design,
		"required: [handler, method]\n      optional: []",
		"required: [method]\n      optional: [handler]", 1)
	source = optionalSumProjectionHandler(source)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		semanticClaimSourceWithLabels("fine-get", "example_requests_total", 2, "method", "GET"),
		semanticClaimSourceWithLabels("fine-post", "example_requests_total", 3, "method", "POST"),
		semanticClaimSourceWithLabels("coarse", "example_request_count_total", 5),
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionClaims() error = %v", err)
	}
}

func TestReconcileProductionClaimsRejectsMixedOptionalIdentityPresence(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "optional: []", `optional: [pid]
    high_cardinality_acceptance:
      operator_value: Per-worker identity preserves multiprocess diagnosis.`, 1)
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  pid_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: Worker identity is present in multiprocess mode.
environment:
`, 1)
	source = strings.Replace(source, "    labels: {}", `    labels:
      pid:
        meaning: Exporter worker process identity.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: operational_population}
        stability: restart_stable
        evidence: [pid_label]`, 1)
	program := compileTestSemanticContract(t, design, source)
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources = []promreplay.SemanticSource{
		{FinalLabels: semanticTestLabels("pid", "100")},
		{},
	}
	reconciled := &ReconciledSemanticCase{Edges: []ReconciledSemanticEdge{
		{SourceIndex: 0, DestinationProfile: "example", View: "requests"},
		{SourceIndex: 1, DestinationProfile: "example", View: "requests"},
	}}
	err = semanticCase.ReconcileProductionClaims(context.Background(), snapshot, reconciled)
	if err == nil || !strings.Contains(err.Error(), `optional identity label "pid" is mixed present/absent`) {
		t.Fatalf("ReconcileProductionClaims() error = %v, want mixed optional-identity failure", err)
	}
}

func stateSources() []promreplay.SemanticSource {
	states := []string{"200", "429", "invalid"}
	out := make([]promreplay.SemanticSource, 0, len(states))
	for index, state := range states {
		source := semanticClaimSource(state, "example_requests_total", 0)
		source.PrometheusType = "gauge"
		if index == 0 {
			source.Value = 1
		}
		source.Labels = semanticTestLabels("status", state)
		source.FinalLabels = semanticTestLabels("status", state)
		out = append(out, source)
	}
	return out
}

func semanticClaimSourceWithLabels(
	occurrenceID, metric string,
	value float64,
	labels ...string,
) promreplay.SemanticSource {
	source := semanticClaimSource(occurrenceID, metric, value)
	for index := 0; index < len(labels); index += 2 {
		source.Labels = append(source.Labels, promreplay.SemanticLabel{Name: labels[index], Value: labels[index+1]})
		source.FinalLabels = append(source.FinalLabels, promreplay.SemanticLabel{Name: labels[index], Value: labels[index+1]})
	}
	return source
}

func sumProjectionContract(withExclusion bool) (string, string) {
	design := strings.Replace(validProfileDesignV1, `  service:
    grain: service
    identity:
      required: []`, `  service:
    grain: HTTP endpoint
    identity:
      required: [handler, method]`, 1)
	if withExclusion {
		design = strings.Replace(design, "exclusions: {}", `exclusions:
  coarse_request_count:
    source: {signal: request_count, components: [total]}
    reason: equivalent_duplicate
    covering_view: requests
    evidence: [request_projection]
    outcome: retain_writable_unrendered`, 1)
	}

	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  request_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:14]
    claim: Handler and method identify request counter children.
  request_projection:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:15]
    claim: The coarse counter equals the fine request population summed by handler.
`, 1)
	source = strings.Replace(source, "    labels: {}", `    labels:
      handler:
        meaning: HTTP handler.
        presence: required
        domain: {kind: closed, values: [a, b]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [request_label]
      method:
        meaning: HTTP method.
        presence: required
        domain: {kind: closed, values: [GET, POST]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [request_label]`, 1)
	coarse := `  request_count:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: example_request_count_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [request_population]
    components:
      total:
        wire_role: scalar
        lifecycle:
          kind: cumulative
          evidence: [request_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed
          evidence: [request_unit]
    labels:
      handler:
        meaning: HTTP handler.
        presence: required
        domain: {kind: closed, values: [a, b]}
        endpoint_cardinality: {kind: closed_domain}
        stability: stable
        evidence: [request_label]
    functional_dependencies: {}
`
	relationship := `relationships:
  request_count_projection:
    kind: sum_projection
    coarse: {signal: request_count, components: [total]}
    fine: {signal: requests, components: [total]}
    group_by: [handler]
    evidence: [request_projection]`
	source = strings.Replace(source, "relationships: {}", coarse+relationship, 1)
	return design, source
}

func optionalSumProjectionHandler(source string) string {
	return strings.ReplaceAll(source,
		"presence: required\n        domain: {kind: closed, values: [a, b]}",
		"presence: optional\n        domain: {kind: closed, values: [a, b]}")
}

func semanticClaimSource(id, metric string, value float64) promreplay.SemanticSource {
	return promreplay.SemanticSource{
		OccurrenceID: id, MetricName: metric, FinalMetricName: metric,
		Component: "scalar", PrometheusType: "counter", Value: value,
	}
}
