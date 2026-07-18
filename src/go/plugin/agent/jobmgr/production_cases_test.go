package jobmgr_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest"
	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/contract"
)

type productionRuntimeCase struct {
	productionCase contract.ProductionCase
	evidence       contract.CaseEvidence
}

func TestProductionAgentCases(t *testing.T) {
	driver := &jobmgrtest.AgentDriver{}
	for caseID, test := range productionCases(t, contract.SuiteAgent) {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				15*time.Second,
			)
			defer cancel()
			if err := driver.Run(
				ctx,
				test.evidence.RuntimePredicate,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProductionProcessCases(t *testing.T) {
	driver := &jobmgrtest.ProcessDriver{}
	for caseID, test := range productionCases(t, contract.SuiteProcess) {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			if err := driver.Run(
				ctx,
				test.evidence.RuntimePredicate,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProductionShippedRootCases(t *testing.T) {
	configDirectory := os.Getenv("JOBMGRTEST_ROOT_CONFIG_DIR")
	driver := productionShippedRootDriver(configDirectory)
	for caseID, test := range productionCases(
		t,
		contract.SuiteShippedRoot,
	) {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			missing, err := driver.RunAvailable(
				ctx,
				test.evidence.RuntimePredicate,
			)
			if err != nil {
				t.Fatal(err)
			}
			if len(missing) != 0 {
				if os.Getenv(
					"JOBMGRTEST_REQUIRE_ALL_ROOTS",
				) == "1" {
					t.Fatalf(
						"required shipped-root binaries disappeared; missing=%v",
						missing,
					)
				}
				t.Skipf(
					"prebuilt supported shipped-root binaries are required; missing=%v",
					missing,
				)
			}
		})
	}
}

func TestProductionShippedRootScenarioMatrix(t *testing.T) {
	driver := productionShippedRootDriver(
		os.Getenv("JOBMGRTEST_ROOT_CONFIG_DIR"),
	)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		30*time.Second,
	)
	defer cancel()
	missing, err := driver.RunMatrixAvailable(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		if os.Getenv("JOBMGRTEST_REQUIRE_ALL_ROOTS") == "1" {
			t.Fatalf(
				"required shipped-root binaries disappeared; missing=%v",
				missing,
			)
		}
		t.Skipf(
			"prebuilt supported shipped-root binaries are required; missing=%v",
			missing,
		)
	}
}

func productionShippedRootDriver(
	configDirectory string,
) jobmgrtest.ShippedRootDriver {
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

func TestProductionCollectorBoundaryCases(t *testing.T) {
	driver := &jobmgrtest.CollectorBoundaryDriver{}
	for caseID, test := range productionCases(
		t,
		contract.SuiteCollectorBoundary,
	) {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			if err := driver.Run(
				ctx,
				test.evidence.RuntimePredicate,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProductionResolverCases(t *testing.T) {
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
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	driver := jobmgrtest.ResolverDriver{
		Helper: helper, PIDFile: pidFile,
	}
	for caseID, test := range productionCases(
		t,
		contract.SuiteResolver,
	) {
		t.Run(caseID, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(
				context.Background(),
				10*time.Second,
			)
			defer cancel()
			if err := driver.Run(
				ctx,
				test.evidence.RuntimePredicate,
			); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func productionCases(
	t *testing.T,
	suite contract.RuntimeSuite,
) map[string]productionRuntimeCase {
	t.Helper()
	cases, err := contract.BMM002Cases()
	if err != nil {
		t.Fatal(err)
	}
	if err := contract.ValidateEvidenceContract(); err != nil {
		t.Fatal(err)
	}
	selected := make(map[string]productionRuntimeCase)
	for _, productionCase := range cases {
		if productionCase.Suite != suite {
			continue
		}
		evidence, err := contract.EvidenceFor(productionCase)
		if err != nil {
			t.Fatalf("case %s: %v", productionCase.ID, err)
		}
		if evidence.RuntimePredicate == "" {
			continue
		}
		selected[productionCase.ID] = productionRuntimeCase{
			productionCase: productionCase,
			evidence:       evidence,
		}
	}
	if len(selected) == 0 {
		t.Fatalf("B-M-002 suite %s has no cases", suite)
	}
	return selected
}

func TestProductionRuntimeCasesExcludeComponentOnlyEvidence(t *testing.T) {
	tests := map[string]struct {
		suite    contract.RuntimeSuite
		excluded string
		included string
	}{
		"collector boundary": {
			suite:    contract.SuiteCollectorBoundary,
			excluded: "F18.4",
			included: "F18.1",
		},
		"Agent": {
			suite:    contract.SuiteAgent,
			excluded: "F03.1",
			included: "F01.1",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cases := productionCases(t, test.suite)
			if _, ok := cases[test.excluded]; ok {
				t.Fatalf(
					"component-only case %s reached runtime driver",
					test.excluded,
				)
			}
			if _, ok := cases[test.included]; !ok {
				t.Fatalf(
					"runtime case %s did not reach runtime driver",
					test.included,
				)
			}
		})
	}
}
