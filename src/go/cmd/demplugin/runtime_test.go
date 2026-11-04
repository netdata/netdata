// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runtimeCommand(t *testing.T, args ...string) (runtimeCommandReport, error) {
	t.Helper()
	var out, diagnostic bytes.Buffer
	handled, err := runRuntimeCommand(context.Background(), args, &out, &diagnostic)
	require.True(t, handled)
	var report runtimeCommandReport
	if out.Len() > 0 {
		require.NoError(t, json.Unmarshal(out.Bytes(), &report), out.String())
	}
	return report, err
}
func TestRuntimeDispatchAndBadArguments(t *testing.T) {
	for _, args := range [][]string{nil, {"-d"}, {"--version"}} {
		var out bytes.Buffer
		handled, err := runRuntimeCommand(context.Background(), args, &out, &out)
		require.NoError(t, err)
		assert.False(t, handled)
		assert.Empty(t, out.String())
	}
	for _, args := range [][]string{
		{"runtime"}, {"runtime", "unknown"}, {"runtime", "status", "stray"}, {"runtime", "install", "--url", "https://arbitrary.invalid"},
		{"runtime", "import"}, {"runtime", "preload"}, {"runtime", "status", "--archive", "/tmp/archive"},
		{"runtime", "install", "--root", "/tmp/root"}, {"runtime", "preload", "--root", "/tmp/root", "--cache", "/tmp/cache"},
		{"runtime", "status", "--assets", "relative"}, {"runtime", "import", "--archive", "relative"},
	} {
		t.Run(args[len(args)-1], func(t *testing.T) { _, err := runtimeCommand(t, args...); require.Error(t, err) })
	}
}
func TestRuntimeMissingReleaseNeverPreparesCandidate(t *testing.T) {
	for _, command := range []string{"install", "update", "repair", "preload"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "cache")
			writeAdminTrust(t, root, "")
			args := []string{"runtime", command, "--config-dir", root, "--release", filepath.Join(root, "missing.json"), "--archive", filepath.Join(root, "archive.tar.zst")}
			if command == "preload" {
				args = append(args, "--root", cache)
			} else {
				args = append(args, "--cache", cache)
			}
			report, err := runtimeCommand(t, args...)
			require.Error(t, err)
			assert.Contains(t, []string{"unknown", "unavailable"}, report.State)
			assert.Nil(t, report.Candidate)
			assert.NotEmpty(t, report.SetupError)
			require.Nil(t, report.Qualification)
			if report.Storage != nil {
				assert.Nil(t, report.Storage.Selected)
				assert.Zero(t, report.Storage.CandidateClaims)
			}
		})
	}
}
func TestRuntimeSelectionCommandsDoNotRequireOriginalDescriptor(t *testing.T) {
	for _, command := range []string{"status", "qualify", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "absent")
			report, err := runtimeCommand(t, "runtime", command, "--config-dir", root, "--cache", cache)
			require.Error(t, err)
			assert.Empty(t, report.ReleasePath)
			assert.Equal(t, cache, report.CachePath)
			assert.NoDirExists(t, cache)
		})
	}
}
func TestRuntimePreloadedCommandsRequirePreparedBundle(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {mode: preloaded, preload_path: /absent/bundle}"), 0600))
	for _, command := range []string{"status", "qualify"} {
		report, err := runtimeCommand(t, "runtime", command, "--config-dir", dir)
		require.Error(t, err)
		assert.Equal(t, "unavailable", report.State)
		require.Nil(t, report.Qualification)
	}
	for _, command := range []string{"install", "update", "repair", "uninstall"} {
		args := []string{"runtime", command, "--config-dir", dir}
		if command != "uninstall" {
			args = append(args, "--archive", "/unused", "--release", "/unused.json")
		}
		_, err := runtimeCommand(t, args...)
		require.ErrorContains(t, err, "mode: managed")
	}
}

func TestRuntimeRepairRequiresTrustedExplicitInput(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   []string
		message string
	}{
		{"missing input", nil, "either --archive or --allow-download"},
		{"offline without release", []string{"--archive", "/runtime.tar.zst"}, "offline archive requires"},
		{"relative manifest", []string{"--release", "relative.json"}, "--release must be absolute"},
		{"retired manifest", []string{"--manifest", "/trusted.json"}, "flag provided but not defined"},
		{"no archive", []string{"--release", "/trusted.json"}, "either --archive or --allow-download"},
		{"two sources", []string{"--release", "/trusted.json", "--archive", "/runtime.tar.zst", "--allow-download"}, "either --archive or --allow-download"},
		{"relative archive", []string{"--release", "/trusted.json", "--archive", "runtime.tar.zst"}, "--archive must be absolute"},
		{"preload root", []string{"--release", "/trusted.json", "--allow-download", "--root", "/preload"}, "--root is only valid for preload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runtimeCommand(t, append([]string{"runtime", "repair"}, tc.flags...)...)
			require.ErrorContains(t, err, tc.message)
		})
	}
}

func TestRuntimeManualUpdatesOnlyChangesInstallAndRepairPolicy(t *testing.T) {
	for _, command := range []string{"update", "preload", "status", "qualify", "uninstall"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"runtime", command, "--manual-updates"}
			if command == "update" || command == "preload" {
				args = append(args, "--release", "/release.json", "--archive", "/archive.tar.zst")
			}
			_, err := runtimeCommand(t, args...)
			require.ErrorContains(t, err, "--manual-updates is only valid for install/repair")
		})
	}
}
