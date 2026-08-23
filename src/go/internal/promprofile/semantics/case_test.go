// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"strings"
	"testing"
)

func TestEvaluateCaseEnvironmentRequiresCompleteExactAssignment(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)

	compiled, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{
		"example": {},
	})
	if err != nil {
		t.Fatalf("EvaluateCaseEnvironment() error = %v", err)
	}
	if got := compiled.ActiveProfiles(); len(got) != 1 || got[0] != "example" {
		t.Fatalf("active profiles = %v, want [example]", got)
	}
	if err := compiled.ValidateObservationTarget("requests#requests", ""); err != nil {
		t.Fatalf("ValidateObservationTarget() error = %v", err)
	}

	tests := map[string]struct {
		environment map[string]map[string]AxisValue
		want        string
	}{
		"missing candidate": {
			environment: map[string]map[string]AxisValue{},
			want:        `active profile "example" has no environment assignment`,
		},
		"extra profile": {
			environment: map[string]map[string]AxisValue{"example": {}, "other": {}},
			want:        `inactive or undeclared profile "other"`,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := program.EvaluateCaseEnvironment(context.Background(), tc.environment)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("EvaluateCaseEnvironment() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestEvaluateCaseEnvironmentDerivesConditionalSupportClosure(t *testing.T) {
	supportDesign := strings.Replace(validProfileDesignV1, "profile: example", "profile: runtime", 1)
	supportDesign = strings.Replace(supportDesign, "match: example_*", "match: runtime_*", 1)
	supportDesign = strings.Replace(supportDesign, "namespace: example", "namespace: runtime", 1)
	supportSource := strings.Replace(validSourceSemanticsV1, "profile: example", "profile: runtime", 1)
	support := compileTestSemanticContract(t, supportDesign, supportSource)

	design := strings.Replace(validProfileDesignV1, "supports: {}", `supports:
    runtime:
      activation: Included when runtime instrumentation is enabled.
      when: {any: [{all: [{axis: mode, op: eq, value: enabled}]}]}`, 1)
	design = strings.Replace(design, "signal: requests", "signal: runtime/requests", 1)
	source := strings.Replace(validSourceSemanticsV1, "environment:\n", `  mode_availability:
    kind: availability
    upstream: exporter
    locations: [metrics.go:24]
    claim: Runtime support is conditionally available.
environment:
`, 1)
	source = strings.Replace(source, "  axes: {}", `  axes:
    mode:
      kind: enum
      values: [disabled, enabled]
      meaning: Runtime support mode.
      evidence: [mode_availability]`, 1)
	contract := loadTestSemanticContract(t, design, source, "", "")
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{
		Contract: contract,
		Supports: map[string]*CompiledSemanticContract{"runtime": support},
	})
	if err != nil {
		t.Fatalf("CompileSemanticContract() error = %v", err)
	}

	enabled := "enabled"
	compiled, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{
		"example": {"mode": {String: &enabled}},
		"runtime": {},
	})
	if err != nil {
		t.Fatalf("EvaluateCaseEnvironment(enabled) error = %v", err)
	}
	if got := compiled.ActiveProfiles(); len(got) != 2 || got[0] != "example" || got[1] != "runtime" {
		t.Fatalf("enabled active profiles = %v", got)
	}
	if err := compiled.ValidateObservationTarget("requests#requests", ""); err != nil {
		t.Fatalf("enabled target error = %v", err)
	}

	disabled := "disabled"
	compiled, err = program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{
		"example": {"mode": {String: &disabled}},
	})
	if err != nil {
		t.Fatalf("EvaluateCaseEnvironment(disabled) error = %v", err)
	}
	if err := compiled.ValidateObservationTarget("requests#requests", ""); err == nil || !strings.Contains(err.Error(), "inactive") {
		t.Fatalf("disabled target error = %v, want inactive", err)
	}
}

func TestCompiledSemanticCaseRejectsUnknownObservationCoordinates(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	compiled, err := program.EvaluateCaseEnvironment(context.Background(), map[string]map[string]AxisValue{"example": {}})
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"unknown#requests", "requests#unknown"} {
		if err := compiled.ValidateObservationTarget(target, ""); err == nil || !strings.Contains(err.Error(), "unknown") {
			t.Fatalf("ValidateObservationTarget(%q) error = %v, want unknown coordinate", target, err)
		}
	}
}
