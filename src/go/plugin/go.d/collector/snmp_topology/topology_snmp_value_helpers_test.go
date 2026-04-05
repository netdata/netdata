// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeSNMPHexText_StripsPrefixesAndQuotes(t *testing.T) {
	require.Equal(t, "00 11 22 33", normalizeSNMPHexText(`"hex-string: 00 11 22 33"`))
	require.Equal(t, "0A14043C", normalizeSNMPHexText("octet string: 0A14043C"))
	require.Equal(t, "abc", normalizeSNMPHexText(`'string: abc'`))
}

func TestDecodeLLDPCapabilities_AndInferCategory(t *testing.T) {
	require.Equal(t,
		[]string{"bridge", "router"},
		decodeLLDPCapabilities("28"),
	)
	require.Equal(t, "router", inferCategoryFromCapabilities([]string{"bridge", "router"}))
	require.Equal(t, "access point", inferCategoryFromCapabilities([]string{"wlanAccessPoint"}))
}
