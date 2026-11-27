// SPDX-License-Identifier: GPL-3.0-or-later

package windows

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNodeStaleAfter(t *testing.T) {
	tests := map[string]struct {
		updateEvery int
		timeout     time.Duration
		want        time.Duration
	}{
		"defaults": {
			updateEvery: defaultUpdateEvery,
			timeout:     defaultTimeout,
			want:        105 * time.Second,
		},
		"timeout longer than the interval": {
			updateEvery: 5,
			timeout:     20 * time.Second,
			want:        35 * time.Second,
		},
		"fractional timeout rounds up": {
			updateEvery: 10,
			timeout:     2500 * time.Millisecond,
			want:        33 * time.Second,
		},
		"whole-second timeout stays exact": {
			updateEvery: 1,
			timeout:     time.Second,
			want:        4 * time.Second,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, test.want, nodeStaleAfter(test.updateEvery, test.timeout))
		})
	}
}
