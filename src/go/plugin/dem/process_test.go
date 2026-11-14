// SPDX-License-Identifier: GPL-3.0-or-later

package dem

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processConfig keeps the managed browser runtime store out of the state
// directory, independent of the build's default mode.
func processConfig(t *testing.T) Config {
	cfg := DefaultConfig()
	cfg.Runtime.Mode = runtimehost.ModeManaged
	cfg.Runtime.CachePath = filepath.Join(t.TempDir(), "runtime")
	return cfg
}

func TestPrivateProcessLeavesLiveDirectoriesUntouched(t *testing.T) {
	varLib := t.TempDir()
	var warnings bytes.Buffer
	process, err := Open(Options{Config: processConfig(t), VarLibDir: varLib, Private: true, Warnings: &warnings})
	require.NoError(t, err)
	for _, module := range []string{"receiver", "rum"} {
		require.Contains(t, process.Collectors, module)
		assert.Nil(t, process.Collectors[module].ActivationDependencies, module)
	}
	// Managed and native modes always have a host, so browser jobs wait for it.
	for _, module := range []string{"journey", "lighthouse"} {
		require.Contains(t, process.Collectors, module)
		require.NotNil(t, process.Collectors[module].ActivationDependencies, module)
		dependencies, err := process.Collectors[module].ActivationDependencies(nil)
		require.NoError(t, err)
		require.Len(t, dependencies, 1)
		assert.Equal(t, runtimehost.DependencyKind, dependencies[0].Kind, module)
		assert.Equal(t, runtimehost.DependencyName, dependencies[0].Name, module)
	}
	require.GreaterOrEqual(t, len(process.Services), 2)
	assert.Same(t, process.Retention, process.Services[0])
	assert.Same(t, process.runtime.Host, process.Services[1])
	services := 2
	if process.runtime.Host.Status().Storage != nil {
		services++ // a managed store also runs automatic update preparation
	}
	assert.Len(t, process.Services, services)
	require.NoError(t, process.Close())
	entries, err := os.ReadDir(varLib)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Empty(t, warnings.String())
}

func TestProcessKeepsLiveStoresUnderStateDirectory(t *testing.T) {
	varLib := t.TempDir()
	process, err := Open(Options{Config: processConfig(t), VarLibDir: varLib, Warnings: io.Discard})
	require.NoError(t, err)
	require.NoError(t, process.Close())
	for _, store := range []string{"journal", "artifacts"} {
		assert.DirExists(t, filepath.Join(varLib, "dem", store))
	}
}

func TestProcessRequiresStateDirectoryOutsidePrivateRuns(t *testing.T) {
	_, err := Open(Options{Config: processConfig(t), Warnings: io.Discard})
	require.ErrorContains(t, err, "initializing DEM history: NETDATA_LIB_DIR is required outside terminal/debug mode")
}
