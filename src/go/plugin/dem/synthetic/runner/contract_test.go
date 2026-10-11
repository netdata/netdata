// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRuntimeManifestMatchesPackageLock(t *testing.T) {
	raw, err := os.ReadFile("assets/package-lock.json")
	require.NoError(t, err)
	var lock struct {
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(raw, &lock))
	require.Positive(t, requiredRuntime.NodeMajorMin)
	require.NotEmpty(t, requiredRuntime.Chromium.Version)
	require.Len(t, requiredRuntime.Packages, 5)
	for name, version := range requiredRuntime.Packages {
		pkg, ok := lock.Packages["node_modules/"+name]
		require.True(t, ok, "runtime package %s is absent from the lock", name)
		assert.Equal(t, pkg.Version, version, "runtime package %s", name)
	}
}
