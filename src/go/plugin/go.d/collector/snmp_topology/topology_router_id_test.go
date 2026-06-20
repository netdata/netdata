// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeTopologyRouterIDDropsUnspecifiedAddresses(t *testing.T) {
	tests := map[string]struct {
		in   string
		want string
	}{
		"ipv4-unspecified": {in: "0.0.0.0"},
		"ipv6-unspecified": {in: "::"},
		"hex-unspecified":  {in: "00000000"},
		"valid-router-id":  {in: " 1.2.3.4 ", want: "1.2.3.4"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, normalizeTopologyRouterID(tc.in))
		})
	}
}
