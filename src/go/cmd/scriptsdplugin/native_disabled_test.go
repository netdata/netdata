// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !scripts_native_dev

package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestNativeAbsentFromProductionRegistry(t *testing.T) {
	_, nagios := collectorapi.DefaultRegistry.Lookup("nagios")
	_, native := collectorapi.DefaultRegistry.Lookup("native")
	require.True(t, nagios)
	require.False(t, native, "native must not be exposed by a production build")
}

func TestProductionIgnoresPackageInventory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scripts.d.packages.yaml"), []byte("invalid inventory"), 0644))
	registry, _, err := configurePackages(multipath.MultiPath{dir}, collectorapi.DefaultRegistry)
	require.NoError(t, err)
	require.Equal(t, collectorapi.DefaultRegistry, registry)
}
