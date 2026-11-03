// SPDX-License-Identifier: GPL-3.0-or-later

package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseErrorContainsLocationWithoutSource(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "unbound.conf")
	require.NoError(t, os.WriteFile(filename, []byte("# header\nprivate_config_marker\n"), 0600))
	_, err := Parse(filename)
	require.Error(t, err)
	assert.Contains(t, err.Error(), filename)
	assert.Contains(t, err.Error(), "line 2")
	assert.Contains(t, err.Error(), "bad syntax")
	assert.NotContains(t, err.Error(), "private_config_marker")
}

func BenchmarkParseConfiguration(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Parse("testdata/valid_include.conf"); err != nil {
			b.Fatal(err)
		}
	}
}
