// SPDX-License-Identifier: GPL-3.0-or-later

package l2topology

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOptionalValueDistinguishesAbsentAndPresentZero(t *testing.T) {
	tests := map[string]struct {
		value OptionalValue[int]
		has   bool
		want  int
	}{
		"absent": {
			value: OptionalValue[int]{},
			has:   false,
			want:  0,
		},
		"present zero": {
			value: OptionalValue[int]{Value: 0, Has: true},
			has:   true,
			want:  0,
		},
		"present value": {
			value: OptionalValue[int]{Value: 42, Has: true},
			has:   true,
			want:  42,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, tt.has, tt.value.Has)
			require.Equal(t, tt.want, tt.value.Value)
		})
	}
}
