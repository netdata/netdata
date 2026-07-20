//go:build !windows

package jobmgrtest

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/jobmgrtest/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRootProtocolObservationReportsEarlyProcessExit(t *testing.T) {
	process, err := runner.Start(runner.Spec{
		Executable: "/bin/sh",
		Arguments:  []string{"-c", "printf failed >&2; exit 7"},
	})
	require.NoError(t, err)
	defer process.Kill()

	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err = newRootProtocolObservation().wait(
		ctx,
		process,
		func(int, int, int) bool { return false },
	)
	require.ErrorContains(t, err, "exited before expected protocol lifecycle")
	require.ErrorContains(t, err, "failed")
	require.NotErrorIs(t, err, context.DeadlineExceeded)
}

func TestShippedRootDriverValidatesConfigsBeforeAvailability(t *testing.T) {
	tests := map[string]struct {
		overridePath string
		payload      string
		removePath   string
		wantError    bool
	}{
		"exact empty jobs": {},
		"missing": {
			removePath: "go.d/testrandom.conf",
			wantError:  true,
		},
		"nonempty jobs": {
			overridePath: "go.d/testrandom.conf",
			payload:      "jobs:\n  - name: unexpected\n",
			wantError:    true,
		},
		"unknown field": {
			overridePath: "go.d/testrandom.conf",
			payload:      "jobs: []\nunexpected: true\n",
			wantError:    true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			configDir := t.TempDir()
			writeEmptyRootConfigs(t, configDir)
			if test.overridePath != "" {
				require.NoError(t, os.WriteFile(
					filepath.Join(configDir, test.overridePath),
					[]byte(test.payload),
					0o600,
				))
			}
			if test.removePath != "" {
				require.NoError(t, os.Remove(
					filepath.Join(configDir, test.removePath),
				))
			}
			driver := ShippedRootDriver{
				ConfigDir: configDir,
			}

			missing, err := driver.RunMatrixAvailable(t.Context())
			if test.wantError {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.ElementsMatch(
				t,
				[]string{"godplugin", "ibmdplugin", "scriptsdplugin"},
				missing,
			)
		})
	}
}

func TestShippedRootDriverHasClosedRootMembership(t *testing.T) {
	driver := ShippedRootDriver{
		GoDPlugin:      "go",
		IBMPlugin:      "ibm",
		ScriptsDPlugin: "scripts",
	}
	observed := make(map[string]shippedRoot)
	for _, root := range driver.roots() {
		observed[root.name] = root
	}

	require.Equal(t, map[string]shippedRoot{
		"godplugin": {
			name:       "godplugin",
			executable: "go",
			module:     "testrandom",
			configFile: "go.d/testrandom.conf",
		},
		"ibmdplugin": {
			name:       "ibmdplugin",
			executable: "ibm",
			module:     "websphere_mp",
			configFile: "ibm.d/websphere_mp.conf",
		},
		"scriptsdplugin": {
			name:       "scriptsdplugin",
			executable: "scripts",
			module:     "nagios",
			configFile: "scripts.d/nagios.conf",
		},
	}, observed)
}

func TestShippedRootDriverHasClosedScenarioMembership(t *testing.T) {
	require.Equal(t, [...]shippedRootScenario{
		shippedRootQuit,
		shippedRootRepeatedHUP,
		shippedRootShutdown,
	}, shippedRootScenarios)
}

func TestRootProtocolObservationPreservesChunkBoundaries(t *testing.T) {
	tests := map[string]struct {
		chunks          []string
		wantPublished   int
		wantWithdrawals int
	}{
		"split publication and withdrawal": {
			chunks: []string{
				`FUNCTION GLOB`,
				"AL \"config\" 30 \"help\" \"tags\" 0xFFFF 1 1\n\n" +
					`FUNCTION_DEL GLOBAL "con`,
				"fig\"\n\n",
			},
			wantPublished:   1,
			wantWithdrawals: 1,
		},
		"other routes do not count": {
			chunks: []string{
				"FUNCTION GLOBAL \"other\" 30 \"help\" \"tags\" 0xFFFF 1 1\n\n",
				"FUNCTION_DEL GLOBAL \"other\"\n\n",
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			observation := newRootProtocolObservation()
			for _, chunk := range test.chunks {
				require.NoError(t, observation.observe([]byte(chunk)))
			}
			published, withdrawn, _ := observation.snapshot()
			require.Equal(t, test.wantPublished, published)
			require.Equal(t, test.wantWithdrawals, withdrawn)
		})
	}
}

func writeEmptyRootConfigs(t *testing.T, configDir string) {
	t.Helper()
	for _, relative := range []string{
		"go.d/testrandom.conf",
		"ibm.d/websphere_mp.conf",
		"scripts.d/nagios.conf",
	} {
		path := filepath.Join(configDir, relative)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte("jobs: []\n"), 0o600))
	}
}
