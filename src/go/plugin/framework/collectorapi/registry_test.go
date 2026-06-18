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

func TestRegister_FunctionOnlyWithoutMethods(t *testing.T) {
	registry := make(Registry)

	// Panic case: FunctionOnly without Methods
	assert.Panics(
		t,
		func() {
			registry.Register("funcOnly", Creator{FunctionOnly: true})
		})
}

func TestRegisterPanicOnMethodsAndJobMethodsConflict(t *testing.T) {
	tests := map[string]struct {
		name    string
		creator Creator
	}{
		"panic when both methods and job methods are set": {
			name: "conflict",
			creator: Creator{
				Methods: func() []funcapi.MethodConfig {
					return nil
				},
				JobMethods: func(RuntimeJob) []funcapi.MethodConfig {
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
				"conflict has both Methods and JobMethods defined (mutually exclusive)",
				func() { registry.Register(tc.name, tc.creator) },
			)
		})
	}
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
