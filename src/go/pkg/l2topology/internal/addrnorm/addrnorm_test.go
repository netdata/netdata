// SPDX-License-Identifier: GPL-3.0-or-later

package addrnorm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMAC(t *testing.T) {
	tests := map[string]string{
		"0:15:99:9f:7:ef":      "00:15:99:9f:07:ef",
		"60:33:4b:8:17:a8":     "60:33:4b:08:17:a8",
		"0:90:1a:42:22:f8":     "00:90:1a:42:22:f8",
		"0011.2233.4455":       "00:11:22:33:44:55",
		"8.234.68.170.187.204": "08:ea:44:aa:bb:cc",
		"not-a-mac":            "",
	}

	for input, expected := range tests {
		t.Run(input, func(t *testing.T) {
			require.Equal(t, expected, NormalizeMAC(input))
		})
	}
}

func TestDecodeHexBytes(t *testing.T) {
	require.Equal(t, []byte{0, 17, 34, 51, 68, 85}, DecodeHexBytes("0011.2233.4455"))
	require.Equal(t, []byte{0, 17, 34, 51, 68, 85}, DecodeHexBytes("0:11:22:33:44:55"))
	require.Nil(t, DecodeHexBytes("not-hex"))
}
