// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest"
	"github.com/stretchr/testify/require"
)

func TestProductionAgentScenarios(t *testing.T) {
	tests := map[string]jobmgrtest.AgentScenario{
		"start acknowledgement":                        jobmgrtest.AgentStartAcknowledgement,
		"start replacement ordering":                   jobmgrtest.AgentStartReplacementOrdering,
		"collector V1 lifecycle":                       jobmgrtest.AgentCollectorV1Lifecycle,
		"collector V2 lifecycle":                       jobmgrtest.AgentCollectorV2Lifecycle,
		"collector V2 acquired abort":                  jobmgrtest.AgentCollectorV2AcquiredAbort,
		"Function admission closes before lease drain": jobmgrtest.AgentFunctionAdmissionClosesBeforeDrain,
		"Function replacement ordering":                jobmgrtest.AgentFunctionReplacementOrdering,
		"collector V1 published abort":                 jobmgrtest.AgentCollectorV1PublishedAbort,
		"blocking stop":                                jobmgrtest.AgentBlockingStop,
		"held handler shutdown":                        jobmgrtest.AgentHeldHandlerShutdown,
		"late handler quarantine":                      jobmgrtest.AgentLateHandlerQuarantine,
		"UID growth":                                   jobmgrtest.AgentUIDGrowth,
		"concurrent same-route Functions":              jobmgrtest.AgentConcurrentSameRouteFunctions,
		"control with large ordinary population":       jobmgrtest.AgentControlWithLargeOrdinaryPopulation,
		"Function terminal variants":                   jobmgrtest.AgentFunctionTerminalVariants,
		"awaiting terminal bound":                      jobmgrtest.AgentAwaitingTerminalBound,
		"Function output fault":                        jobmgrtest.AgentFunctionOutputFault,
		"config output fault":                          jobmgrtest.AgentConfigOutputFault,
		"collector V1 output fault":                    jobmgrtest.AgentCollectorV1OutputFault,
		"collector V2 output fault":                    jobmgrtest.AgentCollectorV2OutputFault,
		"runtime output fault":                         jobmgrtest.AgentRuntimeOutputFault,
		"cleanup output fault":                         jobmgrtest.AgentCleanupOutputFault,
		"Function header boundaries":                   jobmgrtest.AgentFunctionHeaderBoundaries,
		"Function body boundaries":                     jobmgrtest.AgentFunctionBodyBoundaries,
		"Function raw payload":                         jobmgrtest.AgentFunctionRawPayload,
		"Function timeout boundaries":                  jobmgrtest.AgentFunctionTimeoutBoundaries,
		"Function invalid JSON":                        jobmgrtest.AgentFunctionInvalidJSON,
		"Function result boundaries":                   jobmgrtest.AgentFunctionResultBoundaries,
	}
	driver := &jobmgrtest.AgentDriver{}
	for name, scenario := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
			defer cancel()

			require.NoError(t, driver.Run(ctx, scenario))
		})
	}
}

func TestProductionProcessScenarios(t *testing.T) {
	tests := map[string]jobmgrtest.ProcessScenario{
		"restart waits for old Cleanup":              jobmgrtest.ProcessRestart,
		"noncooperative Cleanup remains owned":       jobmgrtest.ProcessNoncooperativeShutdown,
		"input fence cleans up exactly once":         jobmgrtest.ProcessInputFence,
		"repeated stop invokes Cleanup exactly once": jobmgrtest.ProcessRepeatedStop,
	}
	driver := &jobmgrtest.ProcessDriver{}
	for name, scenario := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()

			require.NoError(t, driver.Run(ctx, scenario))
		})
	}
}

func TestProductionResolverContainment(t *testing.T) {
	if !jobmgrtest.ResolverDriverSupported() {
		t.Skip("process-group containment requires a Unix process model")
	}
	directory := t.TempDir()
	helper := filepath.Join(directory, "resolver-helper")
	pidFile := filepath.Join(directory, "resolver-pids")
	script := "#!/bin/sh\n" +
		"sleep 30 &\n" +
		"child=$!\n" +
		"printf '%s %s\\n' \"$$\" \"$child\" > \"$1\"\n" +
		"wait \"$child\"\n"

	require.NoError(t, os.WriteFile(helper, []byte(script), 0o700))

	driver := jobmgrtest.ResolverDriver{
		Helper: helper, PIDFile: pidFile,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	require.NoError(t, driver.Run(ctx))
}

func TestProductionShippedRootScenarioMatrix(t *testing.T) {
	driver := productionShippedRootDriver(t)
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	defer cancel()

	missing, err := driver.RunMatrixAvailable(ctx)
	require.NoError(t, err)
	if len(missing) != 0 {
		require.NotEqualValues(
			t,
			"1",
			os.Getenv("JOBMGRTEST_REQUIRE_ALL_ROOTS"),
		)
		t.Skipf(
			"prebuilt supported shipped-root binaries are required; missing=%v",
			missing,
		)
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
		Roots: map[string]jobmgrtest.ShippedRoot{
			"godplugin": {
				Executable: os.Getenv("JOBMGRTEST_GODPLUGIN_BIN"),
				Module:     "testrandom",
				ConfigDir:  configDirectory,
			},
			"ibmdplugin": {
				Executable: os.Getenv("JOBMGRTEST_IBMDPLUGIN_BIN"),
				Module:     "websphere_mp",
				ConfigDir:  configDirectory,
			},
			"scriptsdplugin": {
				Executable: os.Getenv("JOBMGRTEST_SCRIPTSDPLUGIN_BIN"),
				Module:     "nagios",
				ConfigDir:  configDirectory,
			},
		},
	}
}
