// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSelectDestinations(t *testing.T) {
	cfg := Plan{
		Destinations: map[string]Sender{
			"primary":  nil,
			"shared":   nil,
			"database": nil,
			"fallback": nil,
		},
		Routing: Routing{
			Default: []string{"fallback", "shared"},
			Roles: map[string][]string{
				"sysadmin": {"primary", "shared", "primary"},
				"dba":      {"shared", "database"},
				"muted":    {},
			},
		},
	}
	tests := map[string]struct {
		destination string
		roles       []string
		want        []string
		err         string
	}{
		"explicit": {
			destination: "primary",
			want:        []string{"primary"},
		},
		"unknown explicit": {destination: "missing", err: "not configured"},
		"duplicate targets": {
			roles: []string{"sysadmin"},
			want:  []string{"primary", "shared"},
		},
		"overlapping roles": {
			roles: []string{"sysadmin", "dba", "sysadmin"},
			want:  []string{"primary", "shared", "database"},
		},
		"input order": {
			roles: []string{"dba", "sysadmin"},
			want:  []string{"shared", "database", "primary"},
		},
		"default per missing role": {
			roles: []string{"sysadmin", "missing"},
			want:  []string{"primary", "shared", "fallback"},
		},
		"default deduplication": {
			roles: []string{"missing", "other"},
			want:  []string{"fallback", "shared"},
		},
		"explicit suppression": {roles: []string{"muted"}},
		"reserved roles":       {roles: []string{"silent", "disabled"}},
		"reserved roles do not suppress others": {
			roles: []string{"silent", "dba", "disabled"},
			want:  []string{"shared", "database"},
		},
		"exact case sensitive roles": {
			roles: []string{"DBA"},
			want:  []string{"fallback", "shared"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := cfg.Select(test.destination, test.roles)
			if test.err != "" {
				require.ErrorContains(t, err, test.err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}
