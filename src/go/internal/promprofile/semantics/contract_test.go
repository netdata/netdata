// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAndCompileMinimalSemanticContract(t *testing.T) {
	contract := loadTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1, "", "")
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}
	if program.Profile() != "example" || len(program.signals) != 1 || len(program.registrations) != 1 {
		t.Fatalf("compiled program = %#v", program)
	}
	registration := program.registrations[inlineRegistrationKey("requests", "canonical")]
	if registration == nil || len(registration.owners) != 1 || registration.owners[0].id != "requests" {
		t.Fatalf("compiled registration = %#v", registration)
	}
}

func TestLoadSemanticContractRequiresIdentityAndRegistryPair(t *testing.T) {
	t.Run("profile mismatch", func(t *testing.T) {
		source := strings.Replace(validSourceSemanticsV1, "profile: example", "profile: other", 1)
		directory := t.TempDir()
		designPath := filepath.Join(directory, ProfileDesignFilename)
		sourcePath := filepath.Join(directory, SourceFilename)
		writeTextFile(t, designPath, validProfileDesignV1)
		writeTextFile(t, sourcePath, source)
		_, err := LoadSemanticContract(context.Background(), SemanticContractPaths{
			ProfileDesign:   designPath,
			SourceSemantics: sourcePath,
		})
		if err == nil || !strings.Contains(err.Error(), "profile mismatch") {
			t.Fatalf("LoadSemanticContract() error = %v, want profile mismatch", err)
		}
	})

	t.Run("registry pair", func(t *testing.T) {
		directory := t.TempDir()
		designPath := filepath.Join(directory, ProfileDesignFilename)
		sourcePath := filepath.Join(directory, SourceFilename)
		writeTextFile(t, designPath, validProfileDesignV1)
		writeTextFile(t, sourcePath, validSourceSemanticsV1)
		_, err := LoadSemanticContract(context.Background(), SemanticContractPaths{
			ProfileDesign:   designPath,
			SourceSemantics: sourcePath,
			SourceRegistry:  filepath.Join(directory, SourceRegistryFilename),
		})
		if err == nil || !strings.Contains(err.Error(), "must be present together") {
			t.Fatalf("LoadSemanticContract() error = %v, want pair failure", err)
		}
	})

	t.Run("registry upstream disagreement", func(t *testing.T) {
		generator := strings.Replace(validSourceRegistryGeneratorV1, "repository: owner/exporter", "repository: owner/other", 1)
		directory := t.TempDir()
		designPath := filepath.Join(directory, ProfileDesignFilename)
		sourcePath := filepath.Join(directory, SourceFilename)
		registryPath := filepath.Join(directory, SourceRegistryFilename)
		generatorPath := filepath.Join(directory, SourceRegistryGeneratorFilename)
		writeTextFile(t, designPath, validProfileDesignV1)
		writeTextFile(t, sourcePath, validSourceSemanticsV1)
		writeTextFile(t, registryPath, validSourceRegistryV1)
		writeTextFile(t, generatorPath, generator)
		writeValidGeneratorDirectory(t, directory)
		_, err := LoadSemanticContract(context.Background(), SemanticContractPaths{
			ProfileDesign:           designPath,
			SourceSemantics:         sourcePath,
			SourceRegistry:          registryPath,
			SourceRegistryGenerator: generatorPath,
		})
		if err == nil || !strings.Contains(err.Error(), "disagrees") {
			t.Fatalf("LoadSemanticContract() error = %v, want upstream disagreement", err)
		}
	})
}

func TestCompileSemanticContractGeneratedOwnership(t *testing.T) {
	t.Run("complete single owner", func(t *testing.T) {
		source := generatedSourceSemanticsV1(
			generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
			false,
		)
		contract := loadTestSemanticContract(
			t, validProfileDesignV1, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
		)
		program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err != nil {
			t.Fatalf("CompileSemanticContract() error = %v", err)
		}
		if len(program.registrations) != 2 || len(program.signals["requests"].registrations) != 2 {
			t.Fatalf("compiled generated program = %#v", program)
		}
	})

	t.Run("unowned registration", func(t *testing.T) {
		source := generatedSourceSemanticsV1(
			generatedSignalV1("requests", "", "[operation_latency]"),
			false,
		)
		contract := loadTestSemanticContract(
			t, validProfileDesignV1, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
		)
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "not owned in every active environment") {
			t.Fatalf("CompileSemanticContract() error = %v, want ownership failure", err)
		}
	})

	t.Run("overlapping owners", func(t *testing.T) {
		source := generatedSourceSemanticsV1(
			generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]")+"\n"+
				generatedSignalV1("requests_copy", "", "[operation_latency, operation_write_latency]"),
			false,
		)
		design := strings.Replace(validProfileDesignV1, "signal: requests", "signal: requests_copy", 1)
		contract := loadTestSemanticContract(
			t, design, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
		)
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "overlapping owners") {
			t.Fatalf("CompileSemanticContract() error = %v, want overlapping-owner failure", err)
		}
	})

	t.Run("mutually exclusive owners cover environment", func(t *testing.T) {
		singleWhen := `    availability:
      any:
        - all: [{axis: mode, op: eq, value: single}]`
		multiWhen := `    availability:
      any:
        - all: [{axis: mode, op: eq, value: multi}]`
		source := generatedSourceSemanticsV1(
			generatedSignalV1("requests", singleWhen, "[operation_latency, operation_write_latency]")+"\n"+
				generatedSignalV1("requests_copy", multiWhen, "[operation_latency, operation_write_latency]"),
			true,
		)
		design := strings.Replace(validProfileDesignV1, "signal: requests", "signal: requests_copy", 1)
		contract := loadTestSemanticContract(
			t, design, source, validSourceRegistryV1, validSourceRegistryGeneratorV1,
		)
		if _, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract}); err != nil {
			t.Fatalf("CompileSemanticContract() error = %v", err)
		}
	})
}

func TestCompileSemanticContractRequiresRawBranchesToCoverRegistrationAvailability(t *testing.T) {
	registry := strings.Replace(validSourceRegistryV1,
		"raw_branches: {canonical: {}, embedded: {}}",
		`raw_branches:
          embedded:
            when: {any: [{all: [{axis: mode, op: eq, value: single}]}]}`,
		1,
	)
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		true,
	)
	contract := loadTestSemanticContract(
		t, validProfileDesignV1, source, registry, validSourceRegistryGeneratorV1,
	)
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "raw branches do not cover its availability") {
		t.Fatalf("CompileSemanticContract() error = %v, want raw-branch coverage failure", err)
	}
}

func TestCompileSemanticContractRejectsOverlappingInlineRegistrationLanguage(t *testing.T) {
	content := strings.Replace(validSourceSemanticsV1, `          canonical:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]`, `          canonical:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]
          duplicate:
            family: {exact: example_requests_total}
            prometheus: {type: counter, shape: scalar}
            evidence: [request_registration]`, 1)
	contract := loadTestSemanticContract(t, validProfileDesignV1, content, "", "")
	_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err == nil || !strings.Contains(err.Error(), "overlapping registrations") {
		t.Fatalf("CompileSemanticContract() error = %v, want language-overlap failure", err)
	}
}

func TestCompileSemanticContractRejectsUnusedEvidenceAndSingleUsePolicies(t *testing.T) {
	t.Run("unused evidence", func(t *testing.T) {
		content := strings.Replace(validSourceSemanticsV1, "evidence:\n", `evidence:
  unused_registration:
    kind: registration
    upstream: exporter
    locations: [metrics.go:9]
    claim: This declaration is intentionally not consumed.
`, 1)
		contract := loadTestSemanticContract(t, validProfileDesignV1, content, "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), `source evidence "unused_registration" is unused`) {
			t.Fatalf("CompileSemanticContract() error = %v, want unused-evidence failure", err)
		}
	})

	t.Run("single-use source policy", func(t *testing.T) {
		content := strings.Replace(validSourceSemanticsV1, "component_policies: {}", `component_policies:
  request_total:
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
        evidence: [request_unit]`, 1)
		content = strings.Replace(content, validScalarComponentV1, "    component_policy: request_total", 1)
		contract := loadTestSemanticContract(t, validProfileDesignV1, content, "", "")
		_, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
		if err == nil || !strings.Contains(err.Error(), "reusable policies require at least two") {
			t.Fatalf("CompileSemanticContract() error = %v, want policy-use failure", err)
		}
	})
}

func loadTestSemanticContract(
	t *testing.T,
	design, source, registry, generator string,
) SemanticContract {
	t.Helper()
	directory := t.TempDir()
	paths := SemanticContractPaths{
		ProfileDesign:   filepath.Join(directory, ProfileDesignFilename),
		SourceSemantics: filepath.Join(directory, SourceFilename),
	}
	writeTextFile(t, paths.ProfileDesign, design)
	writeTextFile(t, paths.SourceSemantics, source)
	if registry != "" || generator != "" {
		paths.SourceRegistry = filepath.Join(directory, SourceRegistryFilename)
		paths.SourceRegistryGenerator = filepath.Join(directory, SourceRegistryGeneratorFilename)
		writeTextFile(t, paths.SourceRegistry, registry)
		writeTextFile(t, paths.SourceRegistryGenerator, generator)
		writeValidGeneratorDirectory(t, directory)
	}
	contract, err := LoadSemanticContract(context.Background(), paths)
	if err != nil {
		t.Fatalf("LoadSemanticContract() error = %v", err)
	}
	return contract
}

func generatedSourceSemanticsV1(signals string, withMode bool) string {
	evidence := ""
	environment := "environment:\n  axes: {}\n  policies: {}"
	if withMode {
		evidence = `  mode_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:9]
    claim: The source defines two exclusive process modes.
`
		environment = `environment:
  axes:
    mode:
      kind: enum
      values: [single, multi]
      meaning: Exporter process mode.
      evidence: [mode_availability]
  policies: {}`
	}
	return fmt.Sprintf(`
version: v1
profile: example
upstreams:
  exporter:
    repository: owner/exporter
    commit: 0123456789abcdef0123456789abcdef01234567
evidence:
%s  request_population:
    kind: population
    upstream: exporter
    locations: [metrics.go:11]
    claim: One observation represents one completed request.
  request_lifecycle:
    kind: lifecycle
    upstream: exporter
    locations: [metrics.go:12]
    claim: The value is a current observation.
  request_unit:
    kind: unit
    upstream: exporter
    locations: [metrics.go:13]
    claim: The value measures completed requests.
%s
component_policies: {}
label_policies: {}
signals:
%s
relationships: {}
state_encodings: {}
source_exclusions: {}
`, evidence, environment, signals)
}

func generatedSignalV1(id, availability, registrations string) string {
	return fmt.Sprintf(`  %s:
%s
    source:
      generated:
        registry_groups: [core]
        scope:
          registrations: %s
    population:
      id: completed_requests
      meaning: Completed application requests.
      evidence: [request_population]
    components:
      total:
        wire_role: scalar
        lifecycle:
          kind: current
          evidence: [request_lifecycle]
        unit:
          quantity: count
          base: one
          rate: none
          object: requests
          aspect: completed
          evidence: [request_unit]
    labels: {}
    functional_dependencies: {}`, id, availability, registrations)
}
