// SPDX-License-Identifier: GPL-3.0-or-later

package netdataapi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSanitizeWireValue(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""},
		{"prometheus.work", "prometheus.work"},
		{" work ", " work "},
		{"wo'rk", "work"},
		{"work\\a\\", "work/a/"},
		{"work\na\rb", "work a b"},
		{"wo\x00rk", "work"},
		{"a'\\\n\r\x00界", "a/  界"},
	} {
		t.Run(tc.input, func(t *testing.T) {
			got := SanitizeWireValue(tc.input)
			assert.Equal(t, tc.want, got)
			assert.Equal(t, got, SanitizeWireValue(got), "published values are canonical")
		})
	}
	assert.Zero(t, testing.AllocsPerRun(100, func() { SanitizeWireValue("prometheus.work") }))
}
