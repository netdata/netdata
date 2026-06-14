// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMethodFunctionNames(t *testing.T) {
	tests := map[string]struct {
		method MethodConfig
		want   []string
	}{
		"default module method name": {
			method: MethodConfig{ID: "logs"},
			want:   []string{"mod:logs"},
		},
		"explicit public function name": {
			method: MethodConfig{ID: "logs", FunctionName: "snmp:traps"},
			want:   []string{"snmp:traps"},
		},
		"aliases are appended once": {
			method: MethodConfig{
				ID:      "topology:snmp",
				Aliases: []string{"topology:snmp", "topology:snmp", ""},
			},
			want: []string{"mod:topology:snmp", "topology:snmp"},
		},
		"aliases matching explicit function name are ignored": {
			method: MethodConfig{
				ID:           "logs",
				FunctionName: "snmp:traps",
				Aliases:      []string{"snmp:traps", "snmp:trap-events"},
			},
			want: []string{"snmp:traps", "snmp:trap-events"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, MethodFunctionNames("mod", tc.method))
		})
	}
}
