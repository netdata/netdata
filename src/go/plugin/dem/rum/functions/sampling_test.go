// SPDX-License-Identifier: GPL-3.0-or-later
package functions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPercentKeepsUsefulPrecisionWithoutBinaryNoise(t *testing.T) {
	for _, tc := range []struct {
		rate float64
		want string
	}{
		{0, "0%"}, {1, "100%"}, {0.07, "7%"}, {0.29, "29%"},
		{0.12345, "12.345%"}, {0.00001, "0.001%"}, {1e-9, "1e-07%"}, {1e-17, "1e-15%"},
	} {
		assert.Equal(t, tc.want, percent(tc.rate), "rate %g", tc.rate)
	}
}
