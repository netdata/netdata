// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileSemanticContractCompilesUnixTimestampExclusion(t *testing.T) {
	design := designWithProcessStartExclusionV1()
	program := compileTestSemanticContract(t, design, sourceWithProcessStartTimestampV1())
	if program.exclusions["process_start_epoch"] == nil {
		t.Fatal("compiled process-start exclusion is missing")
	}
	if got := program.signals["process_start"].components["value"].canonicalUnit; got != "seconds since epoch" {
		t.Fatalf("timestamp canonical unit = %q, want seconds since epoch", got)
	}

	wrongSource := strings.Replace(sourceWithProcessStartTimestampV1(),
		"quantity: timestamp\n          base: unix_second",
		"quantity: duration\n          base: second", 1)
	contract := loadTestSemanticContract(t, design, wrongSource, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "is not a current Unix timestamp") {
		t.Fatalf("CompileSemanticContract() error = %v, want machine-closed timestamp failure", err)
	}

	cumulative := strings.Replace(sourceWithProcessStartTimestampV1(),
		"kind: current\n          evidence: [request_lifecycle]\n        unit:\n          quantity: timestamp",
		"kind: cumulative\n          evidence: [request_lifecycle]\n        unit:\n          quantity: timestamp", 1)
	path := writeSourceSemanticsForLoad(t, cumulative)
	if _, err := LoadSourceSemantics(path); err == nil || !strings.Contains(err.Error(), "timestamp quantity cannot have cumulative lifecycle") {
		t.Fatalf("LoadSourceSemantics() error = %v, want cumulative-timestamp failure", err)
	}
}

func TestCompileSemanticContractCompilesMetadataOnlyScalarExclusion(t *testing.T) {
	design, source := metadataOnlyScalarContract()
	program := compileTestSemanticContract(t, design, source)
	if program.exclusions["runtime_metadata"] == nil {
		t.Fatal("compiled metadata-only exclusion is missing")
	}

	tests := map[string]struct {
		source string
		want   string
	}{
		"non-constant lifecycle": {
			source: strings.Replace(source, "kind: constant", "kind: current", 1),
			want:   "is not constant unit-one metadata",
		},
		"non-unit count": {
			source: strings.Replace(source, `quantity: count
          base: one
          rate: none
          object: runtime_metadata`, `quantity: data
          base: byte
          rate: none
          object: runtime_metadata`, 1),
			want: "is not constant unit-one metadata",
		},
		"no metadata label": {
			source: strings.Replace(source, `    labels:
      version:
        meaning: Runtime version.
        presence: required
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: restart_stable
        evidence: [metadata_label]`, "    labels: {}", 1),
			want: "has no metadata labels",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contract := loadTestSemanticContract(t, design, tc.source, "", "")
			_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CompileSemanticContract() error = %v, want %q", err, tc.want)
			}
		})
	}

	t.Run("non-scalar source", func(t *testing.T) {
		design, source := metadataOnlySummaryContract()
		contract := loadTestSemanticContract(t, design, source, "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "is not a scalar") {
			t.Fatalf("CompileSemanticContract() error = %v, want scalar-shape failure", err)
		}
	})
}

func writeSourceSemanticsForLoad(t *testing.T, source string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), SourceFilename)
	writeTextFile(t, path, source)
	return path
}

func TestCompileSemanticContractRejectsViewExclusionOverlap(t *testing.T) {
	design := strings.Replace(designWithProcessStartExclusionV1(),
		"signal: requests\n        components: [total]",
		"signal: process_start\n        components: [value]", 1)
	contract := loadTestSemanticContract(t, design, sourceWithProcessStartTimestampV1(), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `overlaps view "requests" input "requests"`) {
		t.Fatalf("CompileSemanticContract() error = %v, want view/exclusion overlap failure", err)
	}
}

func TestCompileSemanticContractRequiresEquivalentExclusionRelationship(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "exclusions: {}", `exclusions:
  legacy_duplicate:
    source: {signal: legacy_requests, components: [value]}
    reason: equivalent_duplicate
    covering_view: requests
    evidence: [alias_equivalence]
    outcome: retain_writable_unrendered`, 1)
	compileTestSemanticContract(t, design, sourceWithEquivalentLegacySignalV1(true))

	contract := loadTestSemanticContract(t, design, sourceWithEquivalentLegacySignalV1(false), "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "has no source-equivalent relationship") {
		t.Fatalf("CompileSemanticContract() error = %v, want missing-equivalence failure", err)
	}
}

func TestCompileSemanticContractAcceptsSumProjectionExclusion(t *testing.T) {
	design, source := sumProjectionContract(true)
	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAcceptsExhaustivePartitionWholeExclusion(t *testing.T) {
	design, source := sumProjectionContract(true)
	source = strings.Replace(source, `  request_count_projection:
    kind: sum_projection
    coarse: {signal: request_count, components: [total]}
    fine: {signal: requests, components: [total]}
    group_by: [handler]
    evidence: [request_projection]`, `  request_count_projection:
    kind: partition
    whole: {signal: request_count, components: [total]}
    parts:
      - signal: requests
        components: [total]
        where: {any: [{all: [{label: method, op: eq, value: GET}]}]}
      - signal: requests
        components: [total]
        where: {any: [{all: [{label: method, op: eq, value: POST}]}]}
    disjoint: true
    exhaustive: true
    evidence: [request_projection]`, 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAcceptsSumProjectionCoveredByClosedInputPartition(t *testing.T) {
	design, source := sumProjectionContract(true)
	design = strings.Replace(design,
		"required: [handler, method]\n      optional: []",
		"required: [handler]\n      optional: []", 1)
	design = strings.Replace(design, `    inputs:
      requests:
        signal: requests
        components: [total]
    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    inputs:
      get:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: method, op: eq, value: GET}]}]}
      post:
        signal: requests
        components: [total]
        where: {any: [{all: [{label: method, op: eq, value: POST}]}]}
    labels:
      dimensions: {method: {render: input_role}}
      promote: []
      omit: {}`, 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAcceptsFilteredProjectionCoveredByUnfilteredInput(t *testing.T) {
	design, source := sumProjectionContract(true)
	source = strings.Replace(source,
		"    fine: {signal: requests, components: [total]}", `    fine:
      signal: requests
      components: [total]
      where: {any: [{all: [{label: method, op: eq, value: GET}]}]}`, 1)

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractAcceptsCurrentSumProjectionWithOptionalGroupLabel(t *testing.T) {
	design, source := sumProjectionContract(true)
	design = strings.Replace(design,
		"required: [handler, method]\n      optional: []",
		"required: [method]\n      optional: [handler]", 1)
	source = optionalSumProjectionHandler(source)
	source = strings.ReplaceAll(source, "kind: cumulative", "kind: current")

	compileTestSemanticContract(t, design, source)
}

func TestCompileSemanticContractRejectsInvalidSumProjection(t *testing.T) {
	design, source := sumProjectionContract(true)
	tests := map[string]struct {
		source string
		want   string
	}{
		"unknown group label": {
			source: strings.Replace(source, "group_by: [handler]", "group_by: [missing]", 1),
			want:   `group label "missing" must exist on coarse and fine signals`,
		},
		"different populations": {
			source: strings.Replace(source, "id: completed_requests", "id: other_requests", 1),
			want:   "sum_projection signals have different populations",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			contract := loadTestSemanticContract(t, design, tc.source, "", "")
			_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("CompileSemanticContract() error = %v, want %q", err, tc.want)
			}
		})
	}
	t.Run("missing group key", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), SourceFilename)
		writeTextFile(t, path, strings.Replace(source, "    group_by: [handler]\n", "", 1))
		_, err := LoadSourceSemantics(path)
		if err == nil || !strings.Contains(err.Error(), "group_by must not be empty") {
			t.Fatalf("LoadSourceSemantics() error = %v, want missing group_by failure", err)
		}
	})
}

func TestCompileSemanticContractCompilesStateEncoding(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, `    labels:
      dimensions: {}
      promote: []
      omit: {}`, `    labels:
      dimensions:
        status: {render: label_value}
      promote: []
      omit: {}`, 1)
	program := compileTestSemanticContract(t, design, sourceWithStateEncodingV1(false))
	if program.stateEncodings["request_status"] == nil {
		t.Fatal("compiled state encoding is missing")
	}

	withoutEncoding := strings.Replace(sourceWithStateEncodingV1(false), `state_encodings:
  request_status:
    signal: requests
    component: total
    label: status
    states: ["200", "429", invalid]
    encoding: one_hot_exactly_one
    evidence: [request_state_encoding]`, "state_encodings: {}", 1)
	contract := loadTestSemanticContract(t, design, withoutEncoding, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "lacks complete state-encoding coverage") {
		t.Fatalf("CompileSemanticContract() error = %v, want state-coverage failure", err)
	}

	contract = loadTestSemanticContract(t, design, sourceWithStateEncodingV1(true), "", "")
	_, err = CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), `state encodings "duplicate_status" and "request_status" overlap`) {
		t.Fatalf("CompileSemanticContract() error = %v, want overlapping-state failure", err)
	}
}

func TestCompileSemanticContractDerivesUntypedFallbackClassification(t *testing.T) {
	source := strings.Replace(validSourceSemanticsV1,
		"prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: untyped, shape: scalar, classification: counter}", 1)
	program := compileTestSemanticContract(t, validProfileDesignV1, source)
	fallback, ok := program.fallbacks[inlineRegistrationKey("requests", "canonical")]
	if !ok || fallback.classification != "counter" ||
		len(fallback.exactFamilies) != 1 || fallback.exactFamilies[0] != "example_requests_total" || fallback.embedded != nil {
		t.Fatalf("compiled fallback = %#v", fallback)
	}

	typed := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	if len(typed.fallbacks) != 0 {
		t.Fatalf("typed fallback classifications = %#v, want none", typed.fallbacks)
	}
}

func designWithProcessStartExclusionV1() string {
	return strings.Replace(validProfileDesignV1, "exclusions: {}", `exclusions:
  process_start_epoch:
    source: {signal: process_start, components: [value]}
    reason: not_chartable
    lost_question: How long has the process been running?
    required_operation: age_from_unix_epoch
    evidence: [request_lifecycle, request_unit]
    outcome: drop_before_writer`, 1)
}

func sourceWithProcessStartTimestampV1() string {
	signal := `  process_start:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: process_start_time_seconds}
            prometheus: {type: gauge, shape: scalar}
            evidence: [request_registration]
    population:
      id: process_runtime
      meaning: One process runtime.
      evidence: [request_population]
    components:
      value:
        wire_role: scalar
        lifecycle:
          kind: current
          evidence: [request_lifecycle]
        unit:
          quantity: timestamp
          base: unix_second
          rate: none
          object: process_start
          aspect: observed
          evidence: [request_unit]
    labels: {}
    functional_dependencies: {}
`
	return strings.Replace(validSourceSemanticsV1, "relationships: {}", signal+"relationships: {}", 1)
}

func metadataOnlyScalarContract() (string, string) {
	design := strings.Replace(validProfileDesignV1, "exclusions: {}", `exclusions:
  runtime_metadata:
    source: {signal: runtime_metadata, components: [value]}
    reason: metadata_only
    evidence: [metadata_lifecycle, metadata_unit, metadata_label]
    outcome: retain_writable_unrendered`, 1)
	source := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  metadata_registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:30]
    claim: The exporter registers a scalar runtime metadata family.
  metadata_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:31]
    claim: One sample describes the exporter runtime.
  metadata_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:32]
    claim: Runtime metadata is constant for the exporter lifetime.
  metadata_unit:
    kind: unit
    upstream: exporter
    locations: [metrics.go:33]
    claim: The metadata carrier has the conventional unitless value one.
  metadata_label:
    kind: label
    upstream: exporter
    locations: [metrics.go:34]
    claim: The runtime version is carried as metadata.
`, 1)
	signal := `  runtime_metadata:
    source:
      inline:
        registrations:
          canonical:
            family: {exact: example_runtime_metadata}
            prometheus: {type: gauge, shape: scalar}
            evidence: [metadata_registration]
    population:
      id: exporter_runtime
      meaning: The exporter runtime.
      evidence: [metadata_population]
    components:
      value:
        wire_role: scalar
        lifecycle:
          kind: constant
          evidence: [metadata_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: runtime_metadata
          aspect: information
          evidence: [metadata_unit]
    labels:
      version:
        meaning: Runtime version.
        presence: required
        domain: {kind: open}
        endpoint_cardinality: {kind: singleton}
        stability: restart_stable
        evidence: [metadata_label]
    functional_dependencies: {}
`
	return design, strings.Replace(source, "relationships: {}", signal+"relationships: {}", 1)
}

func metadataOnlySummaryContract() (string, string) {
	design, source := metadataOnlyScalarContract()
	design = strings.Replace(design, "components: [value]", "components: [count]", 1)
	source = strings.Replace(source,
		"prometheus: {type: gauge, shape: scalar}",
		"prometheus: {type: summary, shape: summary}", 1)
	source = strings.Replace(source, `    components:
      value:
        wire_role: scalar
        lifecycle:
          kind: constant
          evidence: [metadata_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: runtime_metadata
          aspect: information
          evidence: [metadata_unit]`, `    components:
      count:
        wire_role: summary_count
        lifecycle:
          kind: constant
          evidence: [metadata_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: runtime_metadata
          aspect: information
          evidence: [metadata_unit]
      sum:
        wire_role: summary_sum
        lifecycle:
          kind: constant
          evidence: [metadata_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: runtime_metadata
          aspect: information
          evidence: [metadata_unit]`, 1)
	return design, source
}

func sourceWithStateEncodingV1(duplicate bool) string {
	source := sourceWithStatusLabelV1()
	source = strings.Replace(source,
		"prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: gauge, shape: scalar}", 1)
	source = strings.Replace(source, "kind: cumulative", "kind: current", 1)
	source = strings.Replace(source, `quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed`, `quantity: state
          base: one
          rate: none
          object: request_state
          aspect: active`, 1)
	source = strings.Replace(source, "environment:\n", `  request_state_encoding:
    kind: state_encoding
    upstream: exporter
    locations: [metrics.go:22]
    claim: Exactly one request status is active.
environment:
`, 1)
	body := `  request_status:
    signal: requests
    component: total
    label: status
    states: ["200", "429", invalid]
    encoding: one_hot_exactly_one
    evidence: [request_state_encoding]`
	if duplicate {
		body = `  duplicate_status:
    signal: requests
    component: total
    label: status
    states: ["200", "429", invalid]
    encoding: one_hot_exactly_one
    evidence: [request_state_encoding]
` + body
	}
	return strings.Replace(source, "state_encodings: {}", "state_encodings:\n"+body, 1)
}

func sourceWithEquivalentLegacySignalV1(withRelationship bool) string {
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  alias_equivalence:
    kind: relationship
    upstream: exporter
    locations: [metrics.go:23]
    claim: Legacy and canonical request counters are equal.
environment:
`, 1)
	legacy := `  legacy_requests:
    source:
      inline:
        registrations:
          legacy:
            family: {exact: example_legacy_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [request_population]
    components:
      value:
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
    labels: {}
    functional_dependencies: {}
`
	relationships := "relationships: {}"
	if withRelationship {
		relationships = `relationships:
  request_alias:
    kind: equivalent
    left: {signal: requests, components: [total]}
    right: {signal: legacy_requests, components: [value]}
    evidence: [alias_equivalence]`
	}
	return strings.Replace(source, "relationships: {}", legacy+relationships, 1)
}
