// SPDX-License-Identifier: GPL-3.0-or-later

package funcapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFunctionNames(t *testing.T) {
	tests := map[string]struct {
		method FunctionConfig
		want   []string
	}{
		"default module method name": {
			method: FunctionConfig{ID: "logs"},
			want:   []string{"mod:logs"},
		},
		"explicit public function name": {
			method: FunctionConfig{ID: "logs", FunctionName: "snmp:traps"},
			want:   []string{"snmp:traps"},
		},
		"aliases are appended once": {
			method: FunctionConfig{
				ID:      "topology:snmp",
				Aliases: []string{"topology:snmp", "topology:snmp", ""},
			},
			want: []string{"mod:topology:snmp", "topology:snmp"},
		},
		"aliases matching explicit function name are ignored": {
			method: FunctionConfig{
				ID:           "logs",
				FunctionName: "snmp:traps",
				Aliases:      []string{"snmp:traps", "snmp:trap-events"},
			},
			want: []string{"snmp:traps", "snmp:trap-events"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, FunctionNames("mod", tc.method))
		})
	}
}
