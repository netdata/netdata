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
func TestRuntimeMissingManifestNeverInitializesCache(t *testing.T) {
	for _, command := range []string{"install", "update", "repair", "preload"} {
		t.Run(command, func(t *testing.T) {
			root := t.TempDir()
			cache := filepath.Join(root, "cache")
			args := []string{"runtime", command, "--config-dir", root, "--manifest", filepath.Join(root, "missing.json"), "--archive", filepath.Join(root, "archive.tar.gz")}
			if command == "preload" {
				args = append(args, "--root", cache)
			} else {
				args = append(args, "--cache", cache)
			}
			report, err := runtimeCommand(t, args...)
			require.Error(t, err)
			if command == "repair" {
				assert.Equal(t, "unknown", report.State)
			} else {
				assert.Equal(t, "unavailable", report.State)
			}
			assert.NotEmpty(t, report.SetupError)
			require.Nil(t, report.Qualification)
			_, err = os.Stat(cache)
			assert.True(t, os.IsNotExist(err))
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
			assert.Empty(t, report.ManifestPath)
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
			args = append(args, "--archive", "/unused", "--manifest", "/unused.json")
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
		{"missing manifest", nil, "absolute --manifest"},
		{"relative manifest", []string{"--manifest", "relative.json"}, "absolute --manifest"},
		{"no archive", []string{"--manifest", "/trusted.json"}, "either --archive or --allow-download"},
		{"two sources", []string{"--manifest", "/trusted.json", "--archive", "/runtime.tar.gz", "--allow-download"}, "either --archive or --allow-download"},
		{"relative archive", []string{"--manifest", "/trusted.json", "--archive", "runtime.tar.gz"}, "--archive must be absolute"},
		{"preload root", []string{"--manifest", "/trusted.json", "--allow-download", "--root", "/preload"}, "--root is only valid for preload"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := runtimeCommand(t, append([]string{"runtime", "repair"}, tc.flags...)...)
			require.ErrorContains(t, err, tc.message)
		})
	}
}
