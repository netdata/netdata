// SPDX-License-Identifier: GPL-3.0-or-later

package keyutil

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpaqueCompositeKey(t *testing.T) {
	tests := map[string]struct {
		parts []string
		want  string
	}{
		"empty": {
			want: "",
		},
		"encodes lengths": {
			parts: []string{"a:b", "c"},
			want:  "3:a:b1:c",
		},
		"preserves empty parts": {
			parts: []string{"a", "", "b"},
			want:  "1:a0:1:b",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tc.want, OpaqueCompositeKey(tc.parts...))
		})
	}
}

func TestDeviceIfIndexKey(t *testing.T) {
	require.Equal(t, OpaqueCompositeKey("switch-a", "7"), DeviceIfIndexKey("switch-a", 7))
}
