// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	automaticUpdatesNotice = "Runtime installation enables daily automatic update preparation. Activation requires a DEM restart; use --manual-updates to disable automatic preparation.\n"
	enrollmentNotice       = "First runtime enrollment acquires a signed release and enables daily automatic update preparation. Running DEM processes require a restart to activate the selection.\n"
	statusNotice           = "Selected installation is shown; already running DEM processes may retain an older release until restart. Metadata damage requires explicit repair after stopping and draining consumers."
	unchangedNotice        = "Selected release is unchanged. Already running processes keep their startup release until restart."
	repairFailedNotice     = "Repair failed; selected installation state is shown separately from candidate progress. A cleared selection is not restored. Stop and drain all consumers before retrying repair."
	uninstalledNotice      = "Runtime files removed. This command does not change administrator-owned host policy; netdata-dem-runtime.sh handles high-level policy cleanup."
	alreadySelectedNotice  = "A usable runtime is already selected; no download, qualification or update policy change was performed. A new DEM process uses this selection. Running processes may require a restart; their active release was not inspected."
)

// reportFields returns the report's top-level keys in output order and its scalar values.
func reportFields(t *testing.T, stdout string) ([]string, map[string]any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(stdout))
	token, err := decoder.Token()
	require.NoError(t, err, stdout)
	require.Equal(t, json.Delim('{'), token, stdout)
	var keys []string
	scalars := map[string]any{}
	for decoder.More() {
		token, err := decoder.Token()
		require.NoError(t, err)
		key := token.(string)
		var value any
		require.NoError(t, decoder.Decode(&value))
		keys = append(keys, key)
		switch value.(type) {
		case map[string]any, []any:
		default:
			scalars[key] = value
		}
	}
	return keys, scalars
}

// Each command's report fields, states and notices over the authenticated fixture.
// The fixture never qualifies, so qualification failures carry an
// environment-specific setup_error that is checked for presence only.
func TestRuntimeCommandReportsOverFixture(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure and qualification run as the nonroot service identity")
	}
	type expected struct {
		keys    []string
		scalars map[string]any
		stderr  string
		failed  bool
		// candidate reports the fixture identity as the candidate.
		candidate bool
	}
	for name, tc := range map[string]struct {
		selected bool
		args     func(root, cache, release, archive string) []string
		want     func(cache, generation string) expected
	}{
		"status selected": {
			selected: true,
			args: func(root, cache, _, _ string) []string {
				return []string{"runtime", "status", "--config-dir", root, "--cache", cache}
			},
			want: func(cache, generation string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "cache_path", "generation", "runtime", "storage", "notice"},
					scalars: map[string]any{"download_allowed": false, "command": "status", "mode": "managed", "state": "installed", "cache_path": cache, "generation": generation, "notice": statusNotice},
				}
			},
		},
		"qualify selected": {
			selected: true,
			args: func(root, cache, _, _ string) []string {
				return []string{"runtime", "qualify", "--config-dir", root, "--cache", cache}
			},
			want: func(cache, generation string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "cache_path", "generation", "runtime", "setup_error", "storage", "qualification"},
					scalars: map[string]any{"download_allowed": false, "command": "qualify", "mode": "managed", "state": "installed", "cache_path": cache, "generation": generation},
					failed:  true,
				}
			},
		},
		"uninstall selected": {
			selected: true,
			args: func(root, cache, _, _ string) []string {
				return []string{"runtime", "uninstall", "--config-dir", root, "--cache", cache}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "cache_path", "storage", "notice"},
					scalars: map[string]any{"download_allowed": false, "command": "uninstall", "mode": "managed", "state": "uninstalled", "cache_path": cache, "notice": uninstalledNotice},
				}
			},
		},
		"update unchanged": {
			selected: true,
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "update", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, generation string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "generation", "runtime", "storage", "notice"},
					scalars: map[string]any{"download_allowed": false, "command": "update", "mode": "managed", "state": "installed", "cache_path": cache, "generation": generation, "notice": unchangedNotice},
				}
			},
		},
		"install unchanged": {
			selected: true,
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "install", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, generation string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "generation", "runtime", "storage", "notice"},
					scalars: map[string]any{"download_allowed": false, "command": "install", "mode": "managed", "state": "installed", "cache_path": cache, "generation": generation, "notice": unchangedNotice},
					stderr:  automaticUpdatesNotice,
				}
			},
		},
		"repair selected": {
			selected: true,
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "repair", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:      []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "candidate", "candidate_state", "setup_error", "storage", "qualification", "notice"},
					scalars:   map[string]any{"download_allowed": false, "command": "repair", "mode": "managed", "state": "unavailable", "cache_path": cache, "candidate_state": "prepared", "notice": repairFailedNotice},
					stderr:    automaticUpdatesNotice,
					failed:    true,
					candidate: true,
				}
			},
		},
		"ensure selected": {
			selected: true,
			args: func(root, cache, _, _ string) []string {
				return []string{"runtime", "ensure", "--allow-download", "--config-dir", root, "--cache", cache}
			},
			want: func(cache, generation string) expected {
				return expected{
					keys:    []string{"outcome", "download_allowed", "command", "mode", "state", "cache_path", "generation", "runtime", "notice"},
					scalars: map[string]any{"outcome": "already-selected", "download_allowed": true, "command": "ensure", "mode": "managed", "state": "installed", "cache_path": cache, "generation": generation, "notice": alreadySelectedNotice},
				}
			},
		},
		"install": {
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "install", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:      []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "candidate", "candidate_state", "setup_error", "storage", "qualification"},
					scalars:   map[string]any{"download_allowed": false, "command": "install", "mode": "managed", "state": "unavailable", "cache_path": cache, "candidate_state": "prepared"},
					stderr:    automaticUpdatesNotice,
					failed:    true,
					candidate: true,
				}
			},
		},
		"install with manual updates": {
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "install", "--manual-updates", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:      []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "candidate", "candidate_state", "setup_error", "storage", "qualification"},
					scalars:   map[string]any{"download_allowed": false, "command": "install", "mode": "managed", "state": "unavailable", "cache_path": cache, "candidate_state": "prepared"},
					failed:    true,
					candidate: true,
				}
			},
		},
		"update without installation": {
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "update", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:    []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "setup_error", "storage"},
					scalars: map[string]any{"download_allowed": false, "command": "update", "mode": "managed", "state": "unavailable", "cache_path": cache, "setup_error": "install an authenticated runtime release before checking updates"},
				}
			},
		},
		"preload": {
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "preload", "--config-dir", root, "--root", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:      []string{"download_allowed", "command", "mode", "state", "release_path", "cache_path", "candidate", "candidate_state", "setup_error", "storage", "qualification"},
					scalars:   map[string]any{"download_allowed": false, "command": "preload", "mode": "managed", "state": "unavailable", "cache_path": cache, "candidate_state": "prepared"},
					failed:    true,
					candidate: true,
				}
			},
		},
		"ensure enrollment": {
			args: func(root, cache, release, archive string) []string {
				return []string{"runtime", "ensure", "--config-dir", root, "--cache", cache, "--release", release, "--archive", archive}
			},
			want: func(cache, _ string) expected {
				return expected{
					keys:      []string{"outcome", "download_allowed", "command", "mode", "state", "release_path", "cache_path", "candidate", "candidate_state", "setup_error", "qualification"},
					scalars:   map[string]any{"outcome": "failed", "download_allowed": false, "command": "ensure", "mode": "managed", "state": "unavailable", "cache_path": cache, "candidate_state": "prepared"},
					stderr:    enrollmentNotice,
					failed:    true,
					candidate: true,
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			var root, cache, release, archive string
			var fixture runtime.Manifest
			if tc.selected {
				root, cache, release, archive, fixture = selectedAdminFixture(t)
			} else {
				release, archive = adminBundleFixture(t)
				root = t.TempDir()
				writeAdminTrust(t, root, "")
				cache = filepath.Join(root, "cache")
				trusted, err := runtime.ReadRelease(release, adminTrust(t))
				require.NoError(t, err)
				fixture = trusted.Release().Manifest
			}
			reported := cache
			if strings.HasPrefix(name, "ensure") {
				// ensure reports the canonical root it verified.
				canonical, err := runtime.CanonicalRoot(cache)
				require.NoError(t, err)
				reported = canonical
			}
			want := tc.want(reported, fixture.Archive.SHA256)
			args := tc.args(root, cache, release, archive)
			if slices.Contains(args, "--release") {
				want.scalars["release_path"] = release
			}
			result := runtimeRun(t, context.Background(), args...)
			keys, scalars := reportFields(t, result.stdout)
			assert.Equal(t, want.keys, keys)
			if want.failed {
				assert.NotEmpty(t, scalars["setup_error"])
				want.scalars["setup_error"] = scalars["setup_error"]
			}
			assert.Equal(t, want.scalars, scalars)
			assert.Equal(t, want.stderr, result.stderr)
			if setupError, ok := want.scalars["setup_error"]; ok {
				assert.EqualError(t, result.err, setupError.(string))
			} else {
				assert.NoError(t, result.err)
			}
			var report runtimeCommandReport
			require.NoError(t, json.Unmarshal([]byte(result.stdout), &report))
			if want.candidate {
				require.NotNil(t, report.Candidate)
				assert.Equal(t, fixture.Identity(), *report.Candidate)
			}
			if _, ok := want.scalars["generation"]; ok {
				require.NotNil(t, report.Runtime)
				assert.Equal(t, fixture.Identity(), *report.Runtime)
			}
		})
	}
}
