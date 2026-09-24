// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAtomicValidationClonesAvailableValues(t *testing.T) {
	resolver, err := NewAtomicResolver(map[string]AtomicProvider{
		"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			t.Fatal("validation invoked a provider")
			return nil, nil
		}),
	})
	require.NoError(t, err)
	input := map[string]any{
		"mixed":        "before-${test:value}-after",
		"list":         []string{"${store:vault:missing:key}", "literal", "${inactive}"},
		"map":          map[string]string{"secret": "${test:value}", "ordinary": "kept", "__internal__": "${unknown:literal}"},
		"nested":       []any{map[any]any{"value": "${test:value}", "sibling": 3}},
		"__internal__": map[string]any{"value": "${store:invalid}"},
	}
	value, references, err := resolver.CloneForValidation(input)
	require.NoError(t, err)
	require.True(t, references)
	require.Equal(t, map[string]any{
		"mixed":        nil,
		"list":         []any{nil, "literal", "${inactive}"},
		"map":          map[string]any{"secret": nil, "ordinary": "kept", "__internal__": "${unknown:literal}"},
		"nested":       []any{map[any]any{"value": nil, "sibling": 3}},
		"__internal__": map[string]any{"value": "${store:invalid}"},
	}, value)
	require.Equal(t, "before-${test:value}-after", input["mixed"])
	require.Equal(t, "${store:vault:missing:key}", input["list"].([]string)[0])
	value.(map[string]any)["map"].(map[string]any)["ordinary"] = "changed"
	require.Equal(t, "kept", input["map"].(map[string]string)["ordinary"])
}

func TestAtomicValidationRejectsInvalidReferencesAndBounds(t *testing.T) {
	resolver, err := NewAtomicResolver(map[string]AtomicProvider{
		"test": AtomicProviderFunc(func(context.Context, string) ([]byte, error) {
			t.Fatal("validation invoked a provider")
			return nil, nil
		}),
	})
	require.NoError(t, err)
	cycle := map[string]any{}
	cycle["self"] = cycle
	tests := map[string]struct {
		input any
		kind  AtomicErrorKind
	}{
		"unknown provider":        {"${unknown:key}", AtomicErrorReference},
		"unknown modifier":        {"${test+unknown:key}", AtomicErrorReference},
		"invalid Store reference": {"${store:missing}", AtomicErrorReference},
		"cycle":                   {cycle, AtomicErrorCycle},
		"depth":                   {atomicNestedMap(MaximumAtomicDepth + 1), AtomicErrorDepth},
		"size":                    {strings.Repeat("x", MaximumAtomicResolvedBytes+1), AtomicErrorResultLimit},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			value, _, err := resolver.CloneForValidation(test.input)
			require.Nil(t, value)
			requireAtomicErrorKind(t, err, test.kind)
		})
	}
	value, references, err := resolver.CloneForValidation(strings.Repeat("x", MaximumAtomicResolvedBytes))
	require.NoError(t, err)
	require.False(t, references)
	require.Len(t, value, MaximumAtomicResolvedBytes)
}
