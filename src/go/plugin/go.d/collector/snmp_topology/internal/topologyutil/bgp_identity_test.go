// SPDX-License-Identifier: GPL-3.0-or-later

package topologyutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFirstBGPIdentifier(t *testing.T) {
	for name, tc := range map[string]struct {
		values []string
		want   string
	}{
		"absent":           {},
		"blank":            {values: []string{"", " \t\r\n", "\u2003"}},
		"fallback":         {values: []string{" \t", " edge ", "other"}, want: " edge "},
		"typed precedence": {values: []string{"\r\nedge\t", "edge"}, want: "\r\nedge\t"},
		"default":          {values: []string{" \t", "default"}, want: "default"},
		"zero":             {values: []string{"0", "default"}, want: "0"},
		"control bytes":    {values: []string{" \x00\r\n", "default"}, want: " \x00\r\n"},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, FirstBGPIdentifier(tc.values...))
		})
	}
}
