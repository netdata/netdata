// SPDX-License-Identifier: GPL-3.0-or-later

package policy

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunModeConstructors(t *testing.T) {
	tests := map[string]struct {
		makePolicy func() RunModePolicy
		want       RunModePolicy
	}{
		"agent terminal": {
			makePolicy: func() RunModePolicy { return Agent(true) },
			want: RunModePolicy{
				IsTerminal:               true,
				AutoEnableDiscovered:     true,
				UseFileStatusPersistence: false,
				EnableServiceDiscovery:   false,
				EnableRuntimeCharts:      false,
			},
		},
		"agent daemon": {
			makePolicy: func() RunModePolicy { return Agent(false) },
			want: RunModePolicy{
				IsTerminal:               false,
				AutoEnableDiscovered:     false,
				UseFileStatusPersistence: true,
				EnableServiceDiscovery:   true,
				EnableRuntimeCharts:      true,
			},
		},
		"function cli": {
			makePolicy: FunctionCLI,
			want: RunModePolicy{
				IsTerminal:               false,
				AutoEnableDiscovered:     true,
				UseFileStatusPersistence: true,
				EnableServiceDiscovery:   false,
				EnableRuntimeCharts:      false,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.NotNil(t, test.makePolicy)
			assert.Equal(t, test.want, test.makePolicy())
		})
	}
}
