// SPDX-License-Identifier: GPL-3.0-or-later

package promsemantics

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/promprofile/replay"
)

func TestValidateProductionProfileHeader(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	valid := promreplay.SemanticProfile{Name: "example", Match: "example_*", ContextNamespace: "example"}
	if err := program.ValidateProductionProfileHeader(valid); err != nil {
		t.Fatalf("ValidateProductionProfileHeader() error = %v", err)
	}

	tests := map[string]struct {
		profile promreplay.SemanticProfile
		want    string
	}{
		"match": {
			profile: promreplay.SemanticProfile{Name: "example", Match: "other_*", ContextNamespace: "example"},
			want:    "match got",
		},
		"app presence": {
			profile: promreplay.SemanticProfile{Name: "example", Match: "example_*", HasApp: true, ContextNamespace: "example"},
			want:    "app got present=true",
		},
		"stock selector allow": {
			profile: promreplay.SemanticProfile{
				Name: "example", Match: "example_*", ContextNamespace: "example", AutogenSelectorAllow: []string{"example_*"},
			},
			want: "must not declare autogen.selector.allow",
		},
		"context namespace": {
			profile: promreplay.SemanticProfile{Name: "example", Match: "example_*", ContextNamespace: "other"},
			want:    "context namespace got",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := program.ValidateProductionProfileHeader(tc.profile)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateProductionProfileHeader() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateProductionProfileHeaderAcceptsExactStockAutogenDeny(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	profile := promreplay.SemanticProfile{
		Name: "example", Match: "example_*", ContextNamespace: "example",
		AutogenSelectorDeny: []string{"example_requests_total"},
	}
	if err := program.ValidateProductionProfileHeader(profile); err != nil {
		t.Fatalf("ValidateProductionProfileHeader() error = %v", err)
	}
}

func TestValidateProductionProfileHeaderRejectsInvalidStockAutogenDeny(t *testing.T) {
	program := compileTestSemanticContract(t, validProfileDesignV1, validSourceSemanticsV1)
	tests := map[string]struct {
		deny []string
		want string
	}{
		"wildcard":          {deny: []string{"example_*"}, want: "must name one exact metric family"},
		"label constrained": {deny: []string{`example_requests_total{status="500"}`}, want: "must name one exact metric family"},
		"duplicate":         {deny: []string{"example_requests_total", "example_requests_total"}, want: "duplicates family"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := promreplay.SemanticProfile{
				Name: "example", Match: "example_*", ContextNamespace: "example", AutogenSelectorDeny: tc.deny,
			}
			err := program.ValidateProductionProfileHeader(profile)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateProductionProfileHeader() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateProductionProfileHeaderPreservesAppPresence(t *testing.T) {
	design := strings.Replace(validProfileDesignV1, "namespace: example", "app: service\nnamespace: example", 1)
	program := compileTestSemanticContract(t, design, validSourceSemanticsV1)
	profile := promreplay.SemanticProfile{
		Name: "example", Match: "example_*", App: "service", HasApp: true, ContextNamespace: "example",
	}
	if err := program.ValidateProductionProfileHeader(profile); err != nil {
		t.Fatalf("ValidateProductionProfileHeader() error = %v", err)
	}
	profile.HasApp = false
	if err := program.ValidateProductionProfileHeader(profile); err == nil || !strings.Contains(err.Error(), "present=false") {
		t.Fatalf("ValidateProductionProfileHeader() error = %v, want presence mismatch", err)
	}
}

func TestValidateProductionProfileHeaderReconcilesFallbackMapping(t *testing.T) {
	source := strings.Replace(
		validSourceSemanticsV1,
		"prometheus: {type: counter, shape: scalar}",
		"prometheus: {type: untyped, shape: scalar, classification: counter}",
		1,
	)
	program := compileTestSemanticContract(t, validProfileDesignV1, source)
	valid := promreplay.SemanticProfile{
		Name: "example", Match: "example_*", ContextNamespace: "example",
		FallbackRules: []promreplay.SemanticFallbackRule{{
			RuntimePath: "fallback_type.counter[0]", AssertedType: "counter", Pattern: "example_requests_total",
		}},
	}
	if err := program.ValidateProductionProfileHeader(valid); err != nil {
		t.Fatalf("ValidateProductionProfileHeader() error = %v", err)
	}

	tests := map[string]struct {
		mutate func(*promreplay.SemanticProfile)
		want   string
	}{
		"missing": {
			mutate: func(profile *promreplay.SemanticProfile) { profile.FallbackRules = nil },
			want:   "missing=[example_requests_total=counter]",
		},
		"extra broad pattern": {
			mutate: func(profile *promreplay.SemanticProfile) { profile.FallbackRules[0].Pattern = "example_*" },
			want:   "extra=[example_*=counter]",
		},
		"wrong classification": {
			mutate: func(profile *promreplay.SemanticProfile) { profile.FallbackRules[0].AssertedType = "gauge" },
			want:   "classifies as \"gauge\"",
		},
		"duplicate": {
			mutate: func(profile *promreplay.SemanticProfile) {
				profile.FallbackRules = append(profile.FallbackRules, profile.FallbackRules[0])
			},
			want: "is duplicated",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := valid
			profile.FallbackRules = slices.Clone(valid.FallbackRules)
			tc.mutate(&profile)
			err := program.ValidateProductionProfileHeader(profile)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateProductionProfileHeader() error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestValidateProductionProfileHeaderDerivesGrammarFallbackPatterns(t *testing.T) {
	source := generatedSourceSemanticsV1(
		generatedSignalV1("requests", "", "[operation_latency, operation_write_latency]"),
		false,
	)
	registry := strings.ReplaceAll(
		validSourceRegistryV1,
		"prometheus: {type: gauge, shape: scalar}",
		"prometheus: {type: untyped, shape: scalar, classification: gauge}",
	)
	contract := loadTestSemanticContract(t, validProfileDesignV1, source, registry, validSourceRegistryGeneratorV1)
	program, err := CompileSemanticContract(context.Background(), SemanticCompileInput{Contract: contract})
	if err != nil {
		t.Fatal(err)
	}
	profile := promreplay.SemanticProfile{
		Name: "example", Match: "example_*", ContextNamespace: "example",
		FallbackRules: []promreplay.SemanticFallbackRule{
			{AssertedType: "gauge", Pattern: "example_latency"},
			{AssertedType: "gauge", Pattern: "example_?*_latency"},
			{AssertedType: "gauge", Pattern: "example_write_latency"},
			{AssertedType: "gauge", Pattern: "example_?*_write_latency"},
		},
	}
	if err := program.ValidateProductionProfileHeader(profile); err != nil {
		t.Fatalf("ValidateProductionProfileHeader() error = %v", err)
	}
}
