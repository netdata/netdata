// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo

package as400

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInt64ToCount(t *testing.T) {
	tests := map[string]struct {
		input int64
		want  int
	}{
		"zero":           {input: 0, want: 0},
		"small positive": {input: 42, want: 42},
		"negative":       {input: -1, want: 0},
		"min int64":      {input: math.MinInt64, want: 0},
		"max int":        {input: math.MaxInt, want: math.MaxInt},
		"above max int":  {input: math.MaxInt64, want: math.MaxInt},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, int64ToCount(test.input))
		})
	}
}
