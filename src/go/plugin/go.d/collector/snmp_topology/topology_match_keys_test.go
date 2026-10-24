// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp_topology/internal/topologymodel"
	"github.com/stretchr/testify/require"
)

func TestCanonicalKeyHelpers_DeduplicateAndNormalizeDeterministically(t *testing.T) {
	tests := map[string]struct {
		canonical func([]string) string
		values    []string
		want      string
	}{
		"ip-list": {
			canonical: topologymodel.CanonicalIPListKey,
			values:    []string{"alpha", "10.20.4.60", "0A14043C", "ALPHA"},
			want:      "10.20.4.60,alpha",
		},
		"hardware-list": {
			canonical: topologymodel.CanonicalHardwareListKey,
			values:    []string{"chassis-a", "00:11:22:33:44:55", "0A14043C", "001122334455"},
			want:      "00:11:22:33:44:55,10.20.4.60,chassis-a",
		},
		"string-list": {
			canonical: topologymodel.CanonicalStringListKey,
			values:    []string{"edge-b", " Edge-A ", "edge-a"},
			want:      "edge-a,edge-b",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, tc.canonical(tc.values))
		})
	}
}
