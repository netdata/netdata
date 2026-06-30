// SPDX-License-Identifier: GPL-3.0-or-later

package collectorapi

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	modName := "modName"
	registry := make(Registry)

	// OK case
	assert.NotPanics(
		t,
		func() {
			registry.Register(modName, Creator{})
		})

	_, exist := registry[modName]

	require.True(t, exist)

	// Panic case: duplicate registration
	assert.Panics(
		t,
		func() {
			registry.Register(modName, Creator{})
		})

}

func TestRegister_FunctionOnlyWithoutFunctions(t *testing.T) {
	registry := make(Registry)

	// Panic case: FunctionOnly without Functions
	assert.Panics(
		t,
		func() {
			registry.Register("funcOnly", Creator{FunctionOnly: true})
		})
}

func TestRegisterPanicOnStaticFunctionsAndInstanceFunctionsConflict(t *testing.T) {
	tests := map[string]struct {
		name    string
		creator Creator
	}{
		"panic when both static functions and instance functions are set": {
			name: "conflict",
			creator: Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return nil
				},
				InstanceFunctions: func(RuntimeJob) []funcapi.FunctionConfig {
					return nil
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry := make(Registry)

			assert.PanicsWithValue(
				t,
				"conflict has both static Functions and InstanceFunctions defined (mutually exclusive)",
				func() { registry.Register(tc.name, tc.creator) },
			)
		})
	}
}

func TestRegisterPanicOnDuplicateStaticFunctionID(t *testing.T) {
	tests := map[string]struct {
		creator Creator
	}{
		"duplicate shared function IDs": {
			creator: Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{
						{ID: "logs"},
						{ID: "logs"},
					}
				},
			},
		},
		"duplicate agent function IDs": {
			creator: Creator{
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{
						{ID: "logs"},
						{ID: "logs"},
					}
				},
			},
		},
		"duplicate shared and agent function IDs": {
			creator: Creator{
				SharedFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "logs"}}
				},
				AgentFunctions: func() []funcapi.FunctionConfig {
					return []funcapi.FunctionConfig{{ID: "logs"}}
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry := make(Registry)

			assert.PanicsWithValue(
				t,
				`mod has duplicate static Function ID "logs"`,
				func() { registry.Register("mod", tc.creator) },
			)
		})
	}
}

func TestRegisterAllowsEmptyStaticFunctionID(t *testing.T) {
	registry := make(Registry)

	assert.NotPanics(t, func() {
		registry.Register("mod", Creator{
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: ""}, {ID: ""}}
			},
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: ""}}
			},
		})
	})
}

func TestRegister_InstancePolicy(t *testing.T) {
	tests := map[string]struct {
		creator    Creator
		wantPolicy InstancePolicy
		wantPanic  string
	}{
		"default is per-job": {
			creator:    Creator{},
			wantPolicy: InstancePolicyPerJob,
		},
		"single-instance is accepted": {
			creator:    Creator{InstancePolicy: InstancePolicySingle},
			wantPolicy: InstancePolicySingle,
		},
		"unknown policy panics": {
			creator:   Creator{InstancePolicy: InstancePolicy(99)},
			wantPanic: "invalid has invalid InstancePolicy 99",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			registry := make(Registry)
			if tc.wantPanic != "" {
				assert.PanicsWithValue(t, tc.wantPanic, func() {
					registry.Register("invalid", tc.creator)
				})
				return
			}

			registry.Register("mod", tc.creator)
			got, ok := registry.Lookup("mod")
			require.True(t, ok)
			assert.Equal(t, tc.wantPolicy, got.InstancePolicy)
		})
	}
}
