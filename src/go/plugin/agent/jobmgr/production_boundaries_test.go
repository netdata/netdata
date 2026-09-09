// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/internal/jobmgrtest"
	"github.com/stretchr/testify/require"
)

func TestProductionAgentScenarios(t *testing.T) {
	driver := &jobmgrtest.AgentDriver{}
	for scenario := range jobmgrtest.AgentScenarios() {
		t.Run(string(scenario), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()

			require.NoError(t, driver.Run(ctx, scenario))
		})
	}
}

func TestProductionProcessScenarios(t *testing.T) {
	driver := &jobmgrtest.ProcessDriver{}
	for scenario := range jobmgrtest.ProcessScenarios() {
		t.Run(string(scenario), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			require.NoError(t, driver.Run(ctx, scenario))
		})
	}
}

func TestProductionShippedRootScenarioMatrix(t *testing.T) {
	driver := productionShippedRootDriver(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()

	missing, err := driver.RunMatrixAvailable(ctx)
	require.NoError(t, err)
	if len(missing) != 0 {
		require.NotEqualValues(t, "1", os.Getenv("JOBMGRTEST_REQUIRE_ALL_ROOTS"))
		t.Skipf("prebuilt supported shipped-root binaries are required; missing=%v", missing)
	}
}

func productionShippedRootDriver(t *testing.T) jobmgrtest.ShippedRootDriver {
	t.Helper()
	configDirectory := os.Getenv("JOBMGRTEST_ROOT_CONFIG_DIR")
	if configDirectory == "" {
		var err error
		configDirectory, err = filepath.Abs("testdata/root-config")
		require.NoError(t, err)
	}
	require.True(t, filepath.IsAbs(configDirectory))

	return jobmgrtest.ShippedRootDriver{
		ConfigDir:      configDirectory,
		GoDPlugin:      os.Getenv("JOBMGRTEST_GODPLUGIN_BIN"),
		IBMPlugin:      os.Getenv("JOBMGRTEST_IBMDPLUGIN_BIN"),
		ScriptsDPlugin: os.Getenv("JOBMGRTEST_SCRIPTSDPLUGIN_BIN"),
	}
}
