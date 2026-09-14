// SPDX-License-Identifier: GPL-3.0-or-later

package azureprofiles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadFromDefaultDirs_LoadsAllStockProfiles(t *testing.T) {
	dir := azureProfilesDirFromThisFile()
	require.NotEmpty(t, dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	var want int
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		want++
	}

	catalog, err := LoadFromDefaultDirs()
	require.NoError(t, err)

	assert.Equal(t, want, catalog.Len())
}

func TestLoadFromDefaultDirs_StockProfilesUseSelectorShorthand(t *testing.T) {
	dir := azureProfilesDirFromThisFile()
	require.NotEmpty(t, dir)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)

		for line := range strings.SplitSeq(string(data), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- selector:") {
				continue
			}

			selector := strings.TrimSpace(strings.TrimPrefix(line, "- selector:"))
			if selector == "" || strings.HasPrefix(selector, "{") {
				continue
			}

			assert.NotContainsf(t, selector, ".", "stock profile %q should use selector shorthand, found %q", name, selector)
		}
	}
}

// Catalog caching (load-once, retry-after-failure, disabled-under-test) is now
// provided and tested by pkg/profilecatalog (Cached); it is not re-tested per
// collector.
