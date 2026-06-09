// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModuleRegistryWithSystemdPolicy(t *testing.T) {
	base := collectorapi.Registry{
		"logind": collectorapi.Creator{},
		"other":  collectorapi.Creator{},
	}

	withPolicy := moduleRegistryWithSystemdPolicy(base, 239)
	assert.True(t, withPolicy["logind"].Disabled)
	assert.False(t, withPolicy["other"].Disabled)
	assert.False(t, base["logind"].Disabled, "base registry must remain unchanged")

	withoutPolicy := moduleRegistryWithSystemdPolicy(base, 250)
	assert.False(t, withoutPolicy["logind"].Disabled)
	assert.False(t, withoutPolicy["other"].Disabled)
}

func TestResolveFunctionCLIRequest(t *testing.T) {
	registry := collectorapi.Registry{
		"snmp": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: "topology:snmp", Aliases: []string{"topology:snmp"}}}
			},
		},
		"snmp_traps": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: "logs", FunctionName: "snmp:traps"}}
			},
		},
		"plain": collectorapi.Creator{
			Methods: func() []funcapi.MethodConfig {
				return []funcapi.MethodConfig{{ID: "details"}}
			},
		},
		"nofunc": collectorapi.Creator{},
	}

	tests := map[string]struct {
		functionName string
		wantModule   string
		wantMethod   string
		wantStatus   int
		wantErr      string
	}{
		"canonical module method": {
			functionName: "plain:details",
			wantModule:   "plain",
			wantMethod:   "details",
		},
		"explicit public function name": {
			functionName: "snmp:traps",
			wantModule:   "snmp_traps",
			wantMethod:   "logs",
		},
		"alias public function name": {
			functionName: "topology:snmp",
			wantModule:   "snmp",
			wantMethod:   "topology:snmp",
		},
		"unknown module": {
			functionName: "missing:details",
			wantStatus:   404,
			wantErr:      "unknown module 'missing'",
		},
		"module without functions": {
			functionName: "nofunc:details",
			wantStatus:   404,
			wantErr:      "module 'nofunc' does not expose functions",
		},
		"invalid name": {
			functionName: "invalid",
			wantStatus:   400,
			wantErr:      "invalid function name",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			moduleName, methodID, _, err := resolveFunctionCLIRequest(tc.functionName, registry)
			if tc.wantErr != "" {
				require.Error(t, err)
				assert.Equal(t, tc.wantStatus, functionCLIResolutionStatus(err))
				assert.Contains(t, err.Error(), tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.wantModule, moduleName)
			assert.Equal(t, tc.wantMethod, methodID)
		})
	}
}
