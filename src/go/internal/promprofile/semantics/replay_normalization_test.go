// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestReconcileProductionNormalizationsOwnsOrderedMultiRuleTransformations(t *testing.T) {
	sourceContract := sourceWithStatusNormalizationV1()
	design := designWithNormalizationsV1(`  copy_status:
    kind: label_rename
    source_label: status
    target_label: copied_status
    retain_source: false
  status_class:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: copied_status
    target_label: status_class
    exact: {"429": throttled}
    ranges: []
    missing: {leave_absent: true}
    malformed: {set: malformed}
    unknown: {set: other}
    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
    evidence: [status_normalization]`)
	design = strings.Replace(design, "required: []", "required: [copied_status]", 1)
	design = strings.Replace(design, "      promote: []", "      promote: [status_class]", 1)
	program := compileTestSemanticContract(t, design, sourceContract)
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].Labels = semanticTestLabels("status", "429")
	snapshot.Sources[0].FinalMetricName = snapshot.Sources[0].MetricName
	snapshot.Sources[0].FinalLabels = semanticTestLabels("copied_status", "429", "status_class", "throttled")
	snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
		semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
			snapshot.Sources[0].MetricName, semanticTestLabels("status", "429"),
			snapshot.Sources[0].MetricName, semanticTestLabels("copied_status", "429", "status", "429")),
		semanticTestRelabel("relabeling[0].metric_relabel_configs[1]", "labeldrop",
			snapshot.Sources[0].MetricName, semanticTestLabels("copied_status", "429", "status", "429"),
			snapshot.Sources[0].MetricName, semanticTestLabels("copied_status", "429")),
		semanticTestRelabel("relabeling[1].metric_relabel_configs[0]", "replace",
			snapshot.Sources[0].MetricName, semanticTestLabels("copied_status", "429"),
			snapshot.Sources[0].MetricName, semanticTestLabels("copied_status", "429", "status_class", "throttled")),
	}

	reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
	if len(reconciled.Normalizations) != 2 {
		t.Fatalf("normalizations = %#v, want two", reconciled.Normalizations)
	}
	byID := make(map[string]ReconciledSemanticNormalization)
	for _, fact := range reconciled.Normalizations {
		byID[fact.Normalization] = fact
	}
	if got := byID["copy_status"]; got.Branch != "present" ||
		!slices.Equal(got.RuntimePaths, []string{
			"relabeling[0].metric_relabel_configs[0]",
			"relabeling[0].metric_relabel_configs[1]",
		}) {
		t.Fatalf("copy fact = %#v", got)
	}
	if got := byID["status_class"]; got.Branch != "exact:429" ||
		!slices.Equal(got.RuntimePaths, []string{"relabeling[1].metric_relabel_configs[0]"}) {
		t.Fatalf("category fact = %#v", got)
	}
}

func TestReconcileProductionNormalizationsRejectsUnownedAndMisorderedMutations(t *testing.T) {
	sourceContract := sourceWithStatusNormalizationV1()
	design := designWithNormalizationsV1(`  copy_status:
    kind: label_rename
    source_label: status
    target_label: copied_status
    retain_source: true
  status_class:
    kind: category
    applies_to: {signal: requests, components: [total]}
    source_label: copied_status
    target_label: status_class
    exact: {"429": throttled}
    ranges: []
    missing: {leave_absent: true}
    malformed: {set: malformed}
    unknown: {set: other}
    output:
      meaning: HTTP response-status class.
      evidence: [status_class_label]
    evidence: [status_normalization]`)
	design = strings.Replace(design, "required: []", "required: [status]", 1)
	design = strings.Replace(design, "      promote: []", "      promote: [copied_status, status_class]", 1)
	program := compileTestSemanticContract(t, design, sourceContract)

	t.Run("one rule combines purposes", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].Labels = semanticTestLabels("status", "429")
		snapshot.Sources[0].FinalMetricName = snapshot.Sources[0].MetricName
		snapshot.Sources[0].FinalLabels = semanticTestLabels(
			"status", "429", "copied_status", "429", "status_class", "throttled")
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
			semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "labelmap",
				snapshot.Sources[0].MetricName, semanticTestLabels("status", "429"),
				snapshot.Sources[0].MetricName, snapshot.Sources[0].FinalLabels),
		}
		err := reconcileTestProductionNormalizationsError(t, program, snapshot)
		if !strings.Contains(err.Error(), "combines normalization purposes") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("dependency order", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].Labels = semanticTestLabels("status", "429")
		snapshot.Sources[0].FinalMetricName = snapshot.Sources[0].MetricName
		snapshot.Sources[0].FinalLabels = semanticTestLabels(
			"status", "429", "copied_status", "429", "status_class", "throttled")
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
			semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
				snapshot.Sources[0].MetricName, semanticTestLabels("status", "429"),
				snapshot.Sources[0].MetricName, semanticTestLabels("status", "429", "status_class", "throttled")),
			semanticTestRelabel("relabeling[1].metric_relabel_configs[0]", "replace",
				snapshot.Sources[0].MetricName, semanticTestLabels("status", "429", "status_class", "throttled"),
				snapshot.Sources[0].MetricName, snapshot.Sources[0].FinalLabels),
		}
		err := reconcileTestProductionNormalizationsError(t, program, snapshot)
		if !strings.Contains(err.Error(), `normalization "status_class" mutates before dependency "copy_status" completes`) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("unowned field", func(t *testing.T) {
		plain := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].FinalMetricName = snapshot.Sources[0].MetricName
		snapshot.Sources[0].FinalLabels = semanticTestLabels("unexpected", "value")
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
			semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
				snapshot.Sources[0].MetricName, nil,
				snapshot.Sources[0].MetricName, snapshot.Sources[0].FinalLabels),
		}
		err := reconcileTestProductionNormalizationsError(t, plain, snapshot)
		if !strings.Contains(err.Error(), `mutates unowned field "unexpected"`) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestReconcileProductionNormalizationsExtractsEmbeddedIdentity(t *testing.T) {
	sourceContract := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	sourceContract = strings.Replace(sourceContract, "evidence:\n", `evidence:
  worker_identity:
    kind: identity
    upstream: exporter
    locations: [metrics.go:14]
    claim: The worker identity is embedded in each generated family name.
  worker_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: The embedded worker identifies a restart-stable operational process.
`, 1)
	design := designWithNormalizationsV1(`  worker_identity:
    kind: embedded_identity_extract
    registry_grammar: operation_family
    target_label: worker
    retain: {family: canonical_branch, captured_identity: true}
    output:
      meaning: Exporter worker identity.
      endpoint_cardinality: {kind: operational_population}
      stability: restart_stable
      evidence: [worker_label]
    evidence: [worker_identity]`)
	design = strings.Replace(design, `    identity:
      required: []
      optional: []`, `    identity:
      required: []
      optional: [worker]
    high_cardinality_acceptance:
      operator_value: Preserves per-worker diagnosis.`, 1)
	program := compileTestGeneratedSemanticContract(t, design, sourceContract)

	t.Run("embedded", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "example_worker_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		snapshot.Sources[0].FinalMetricName = "example_latency"
		snapshot.Sources[0].FinalLabels = semanticTestLabels("worker", "worker")
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
			semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
				"example_worker_latency", nil,
				"example_worker_latency", semanticTestLabels("worker", "worker")),
			semanticTestRelabel("relabeling[0].metric_relabel_configs[1]", "replace",
				"example_worker_latency", semanticTestLabels("worker", "worker"),
				"example_latency", semanticTestLabels("worker", "worker")),
		}
		reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
		fact := reconciled.Normalizations[0]
		if fact.Branch != "embedded" || len(fact.RuntimePaths) != 2 || fact.Terminal {
			t.Fatalf("normalization fact = %#v", fact)
		}
	})

	t.Run("canonical", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "example_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		snapshot.Sources[0].FinalMetricName = "example_latency"
		reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
		fact := reconciled.Normalizations[0]
		if fact.Branch != "canonical" || len(fact.RuntimePaths) != 0 || fact.Terminal {
			t.Fatalf("normalization fact = %#v", fact)
		}
	})
}

func TestReplayEmbeddedNormalizationBranchSupportsTerminalIdentity(t *testing.T) {
	binding := ReconciledSemanticSource{entry: compiledSourceEntry{
		formID: "identity",
		canonical: &GrammarAffix{
			Prefix: "ceph_service_unique_id",
		},
		embedded: &GrammarEmbedded{
			Prefix: "ceph_service_unique_id_",
		},
	}}

	branch, capture, canonical, err := replayEmbeddedNormalizationBranch(
		binding, "ceph_service_unique_id_service_a", "scalar",
	)
	if err != nil {
		t.Fatalf("replayEmbeddedNormalizationBranch() error = %v", err)
	}
	if branch != "embedded" || capture != "service_a" || canonical != "ceph_service_unique_id" {
		t.Fatalf("branch=%q capture=%q canonical=%q", branch, capture, canonical)
	}

	if _, _, _, err := replayEmbeddedNormalizationBranch(
		binding, "ceph_service_unique_id_", "scalar",
	); err == nil || !strings.Contains(err.Error(), "empty embedded identity") {
		t.Fatalf("empty terminal identity error = %v", err)
	}
}

func TestReplayEmbeddedNormalizationBranchRejectsExcludedNamespace(t *testing.T) {
	binding := ReconciledSemanticSource{entry: compiledSourceEntry{
		formID: "latency",
		canonical: &GrammarAffix{
			Prefix: "example_",
			Suffix: "latency",
		},
		embedded: &GrammarEmbedded{
			Prefix:           "example_",
			ExcludedPrefixes: []string{"example_special_"},
			Separator:        "_",
			Suffix:           "latency",
		},
	}}

	if _, _, _, err := replayEmbeddedNormalizationBranch(
		binding, "example_special_worker_latency", "scalar",
	); err == nil || !strings.Contains(err.Error(), "outside grammar form") {
		t.Fatalf("excluded namespace error = %v", err)
	}
}

func TestReconcileProductionNormalizationsRepairsOrDropsEmbeddedIdentity(t *testing.T) {
	sourceContract := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	sourceContract = strings.Replace(sourceContract, "evidence:\n", `evidence:
  zone_identity:
    kind: identity
    upstream: exporter
    locations: [metrics.go:14]
    claim: The family residue and source label jointly identify the source zone.
  zone_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:15]
    claim: The source-zone label is a restart-stable operational identity.
  duplicate_alias:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:16]
    claim: An embedded row without the source-zone label duplicates the canonical source.
`, 1)
	sourceContract = strings.Replace(sourceContract, "    labels: {}", `    labels:
      source_zone:
        meaning: Source zone fragment.
        presence: optional
        domain: {kind: open}
        endpoint_cardinality: {kind: operational_population}
        stability: restart_stable
        evidence: [zone_label]`, 1)
	design := designWithNormalizationsV1(`  zone_identity:
    kind: embedded_identity_repair
    registry_grammar: operation_family
    source_identity_label: source_zone
    canonical: {family_prefix: example_, identity_label: source_zone}
    embedded: {family_prefix: example_, capture: worker}
    identity:
      operands: [worker, source_zone]
      separator: _
      blank: omit_operand_and_separator
      sanitizer: prometheus_label_value
    duplicate_exclusion:
      when_identity_label: absent
      outcome: drop_before_writer
      evidence: [duplicate_alias]
    output:
      meaning: Canonical source-zone identity reconstructed from the source family and label.
      endpoint_cardinality: {kind: operational_population}
      stability: restart_stable
      evidence: [zone_label]
    evidence: [zone_identity]`)
	design = strings.Replace(design, `    identity:
      required: []
      optional: []`, `    identity:
      required: []
      optional: [source_zone]
    high_cardinality_acceptance:
      operator_value: Preserves per-zone diagnosis.`, 1)
	program := compileTestGeneratedSemanticContract(t, design, sourceContract)

	t.Run("repair", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "example_zone_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		snapshot.Sources[0].Labels = semanticTestLabels("source_zone", "west")
		snapshot.Sources[0].FinalMetricName = "example_latency"
		snapshot.Sources[0].FinalLabels = semanticTestLabels("source_zone", "zone_west")
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
			semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
				"example_zone_latency", semanticTestLabels("source_zone", "west"),
				"example_zone_latency", semanticTestLabels("source_zone", "west", "worker", "zone")),
			semanticTestRelabel("relabeling[0].metric_relabel_configs[1]", "replace",
				"example_zone_latency", semanticTestLabels("source_zone", "west", "worker", "zone"),
				"example_zone_latency", semanticTestLabels("source_zone", "zone_west", "worker", "zone")),
			semanticTestRelabel("relabeling[0].metric_relabel_configs[2]", "replace",
				"example_zone_latency", semanticTestLabels("source_zone", "zone_west", "worker", "zone"),
				"example_latency", semanticTestLabels("source_zone", "zone_west", "worker", "zone")),
			semanticTestRelabel("relabeling[0].metric_relabel_configs[3]", "labeldrop",
				"example_latency", semanticTestLabels("source_zone", "zone_west", "worker", "zone"),
				"example_latency", semanticTestLabels("source_zone", "zone_west")),
		}
		reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
		fact := reconciled.Normalizations[0]
		if fact.Branch != "embedded:identity_present" || len(fact.RuntimePaths) != 4 || fact.Terminal {
			t.Fatalf("normalization fact = %#v", fact)
		}
	})

	t.Run("drop duplicate", func(t *testing.T) {
		snapshot := validProductionSourceSnapshot()
		snapshot.Sources[0].MetricName = "example_zone_latency"
		snapshot.Sources[0].PrometheusType = "gauge"
		snapshot.Sources[0].FinalMetricName = "example_zone_latency"
		drop := semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "drop",
			"example_zone_latency", nil, "example_zone_latency", nil)
		drop.Dropped = true
		snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{drop}
		snapshot.Sources[0].Terminal = &promreplay.SemanticTerminal{
			Disposition: "profile_excluded", Profile: "example", RuntimePath: drop.RuntimePath,
		}
		reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
		fact := reconciled.Normalizations[0]
		if fact.Branch != "embedded:identity_absent" || !fact.Terminal ||
			!slices.Equal(fact.RuntimePaths, []string{drop.RuntimePath}) {
			t.Fatalf("normalization fact = %#v", fact)
		}
		semanticCase, err := program.EvaluateCaseEnvironment(
			context.Background(), map[string]map[string]AxisValue{"example": {}},
		)
		if err != nil {
			t.Fatal(err)
		}
		if err := semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled); err != nil {
			t.Fatalf("ReconcileProductionRoutes() error = %v", err)
		}
		if len(reconciled.Edges) != 0 {
			t.Fatalf("edges = %#v, want none for a normalization-terminal source", reconciled.Edges)
		}
		snapshot.Sources[0].WriterSeries = 1
		err = semanticCase.ReconcileProductionRoutes(context.Background(), snapshot, reconciled)
		if err == nil || !strings.Contains(err.Error(), "terminal reached route/writer state") {
			t.Fatalf("ReconcileProductionRoutes() error = %v, want terminal-leakage failure", err)
		}
	})
}

func TestReconcileProductionNormalizationsOwnsGeneratedComponentExclusion(t *testing.T) {
	sourceContract := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	sourceContract = strings.Replace(sourceContract, "evidence:\n", `evidence:
  generated_registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:10]
    claim: The generator registers the terminal component family class.
`, 1)
	design := designWithNormalizationsV1(`  generated_component:
    kind: generated_component_exclusion
    source:
      namespace_prefix: example_
      terminal_suffix: _write_latency
      component: scalar
    outcome: drop_before_writer
    evidence: [generated_registration, request_lifecycle, request_unit]`)
	program := compileTestGeneratedSemanticContract(t, design, sourceContract)
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].MetricName = "example_write_latency"
	snapshot.Sources[0].PrometheusType = "gauge"
	snapshot.Sources[0].FinalMetricName = "example_write_latency"
	drop := semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "drop",
		"example_write_latency", nil, "example_write_latency", nil)
	drop.Dropped = true
	snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{drop}
	snapshot.Sources[0].Terminal = &promreplay.SemanticTerminal{
		Disposition: "profile_excluded", Profile: "example", RuntimePath: drop.RuntimePath,
	}
	reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
	fact := reconciled.Normalizations[0]
	if fact.Normalization != "generated_component" || fact.Branch != "generated_member" || !fact.Terminal {
		t.Fatalf("normalization fact = %#v", fact)
	}
}

func TestReconcileProductionNormalizationsCanonicalizesFiniteAlias(t *testing.T) {
	sourceContract := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  mode_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:8]
    claim: Exactly one request-family spelling is registered in each exporter mode.
  alias_equivalence:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:9]
    claim: The legacy and canonical families expose the same request signal.
`, 1)
	sourceContract = strings.Replace(sourceContract, `environment:
  axes: {}
  policies: {}`, `environment:
  axes:
    mode:
      kind: enum
      values: [canonical, legacy]
      meaning: Exporter family spelling mode.
      evidence: [mode_availability]
  policies:
    canonical_mode:
      when: {any: [{all: [{axis: mode, op: eq, value: canonical}]}]}
      evidence: [mode_availability]
    legacy_mode:
      when: {any: [{all: [{axis: mode, op: eq, value: legacy}]}]}
      evidence: [mode_availability]`, 1)
	sourceContract = strings.Replace(sourceContract, `          canonical:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]`, `          canonical:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            when: canonical_mode
            evidence: [request_registration]
          legacy:
            family: {exact: example_requests_legacy_total}
            prometheus: {type: counter, shape: scalar}
            when: legacy_mode
            evidence: [request_registration]`, 1)
	design := designWithNormalizationsV1(`  request_alias:
    kind: finite_alias
    applies_to: {signal: requests, components: [total]}
    source_family:
      example_requests_total: example_requests_total
      example_requests_legacy_total: example_requests_total
    evidence: [alias_equivalence]`)
	program := compileTestSemanticContract(t, design, sourceContract)
	legacy := "legacy"
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{
		"example": {"mode": {String: &legacy}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validProductionSourceSnapshot()
	snapshot.Sources[0].MetricName = "example_requests_legacy_total"
	snapshot.Sources[0].FinalMetricName = "example_requests_total"
	snapshot.Sources[0].RelabelRules = []promreplay.SemanticRelabelOccurrence{
		semanticTestRelabel("relabeling[0].metric_relabel_configs[0]", "replace",
			"example_requests_legacy_total", nil, "example_requests_total", nil),
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	if err := semanticCase.ReconcileProductionNormalizations(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionNormalizations() error = %v", err)
	}
	fact := reconciled.Normalizations[0]
	if fact.Normalization != "request_alias" || fact.Branch != "family:example_requests_legacy_total" ||
		len(fact.RuntimePaths) != 1 {
		t.Fatalf("normalization fact = %#v", fact)
	}
}

func TestReconcileProductionNormalizationsOwnsRegistryBackedNamespaceAlias(t *testing.T) {
	design, source, registry := namespaceAliasTestContract()
	contract := loadTestSemanticContract(
		t,
		design,
		source,
		registry,
		validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	raw := "ray_requests"
	canonical := "example_requests"
	snapshot := &promreplay.SemanticSnapshot{
		ContextRoot:      "example",
		SelectedProfiles: []string{"example"},
		Profiles: []promreplay.SemanticProfile{{
			Name: "example", Match: "example_* ray_*", ContextNamespace: "example",
		}},
		Sources: []promreplay.SemanticSource{{
			OccurrenceID:    "ray-request",
			MetricName:      raw,
			FinalMetricName: canonical,
			Component:       "scalar",
			PrometheusType:  "gauge",
			RelabelRules: []promreplay.SemanticRelabelOccurrence{
				semanticTestRelabel(
					"relabeling[0].metric_relabel_configs[0]",
					"replace",
					raw,
					nil,
					canonical,
					nil,
				),
			},
		}},
	}

	reconciled := reconcileTestProductionNormalizations(t, program, snapshot)
	if len(reconciled.Normalizations) != 1 {
		t.Fatalf("normalizations = %#v, want one", reconciled.Normalizations)
	}
	fact := reconciled.Normalizations[0]
	if fact.Normalization != "ray_namespace" || fact.Branch != "family:ray_requests" ||
		!slices.Equal(fact.RuntimePaths, []string{"relabeling[0].metric_relabel_configs[0]"}) {
		t.Fatalf("normalization fact = %#v", fact)
	}
}

func reconcileTestProductionNormalizations(
	t *testing.T,
	program *CompiledSemanticContract,
	snapshot *promreplay.SemanticSnapshot,
) *ReconciledSemanticCase {
	t.Helper()
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	if err := semanticCase.ReconcileProductionNormalizations(context.Background(), snapshot, reconciled); err != nil {
		t.Fatalf("ReconcileProductionNormalizations() error = %v", err)
	}
	return reconciled
}

func compileTestGeneratedSemanticContract(t *testing.T, design, source string) *CompiledSemanticContract {
	t.Helper()
	contract := loadTestSemanticContract(
		t, design, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
	)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	return program
}

func reconcileTestProductionNormalizationsError(
	t *testing.T,
	program *CompiledSemanticContract,
	snapshot *promreplay.SemanticSnapshot,
) error {
	t.Helper()
	semanticCase, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := semanticCase.ReconcileProductionSources(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ReconcileProductionSources() error = %v", err)
	}
	err = semanticCase.ReconcileProductionNormalizations(context.Background(), snapshot, reconciled)
	if err == nil {
		t.Fatal("ReconcileProductionNormalizations() succeeded, want error")
	}
	return err
}

func semanticTestRelabel(
	path, action, inputName string,
	inputLabels []promreplay.SemanticLabel,
	outputName string,
	outputLabels []promreplay.SemanticLabel,
) promreplay.SemanticRelabelOccurrence {
	return promreplay.SemanticRelabelOccurrence{
		Profile:          "example",
		RuntimePath:      path,
		Action:           action,
		Matched:          true,
		InputMetricName:  inputName,
		InputLabels:      inputLabels,
		OutputMetricName: outputName,
		OutputLabels:     outputLabels,
	}
}

func semanticTestLabels(values ...string) []promreplay.SemanticLabel {
	out := make([]promreplay.SemanticLabel, 0, len(values)/2)
	for index := 0; index < len(values); index += 2 {
		out = append(out, promreplay.SemanticLabel{Name: values[index], Value: values[index+1]})
	}
	slices.SortFunc(out, func(left, right promreplay.SemanticLabel) int {
		return strings.Compare(left.Name, right.Name)
	})
	return out
}
