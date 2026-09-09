// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDecodeLLDPCapabilities(t *testing.T) {
	require.Equal(t,
		[]string{"bridge", "router"},
		decodeLLDPCapabilities("28"),
	)
}

func TestInferCategoryFromCapabilities(t *testing.T) {
	tests := map[string]struct {
		capabilities []string
		want         string
	}{
		"access-point": {capabilities: []string{"wlanAccessPoint"}, want: "access point"},
		"router":       {capabilities: []string{"bridge", "router"}, want: "router"},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, inferCategoryFromCapabilities(tc.capabilities))
		})
	}
}
