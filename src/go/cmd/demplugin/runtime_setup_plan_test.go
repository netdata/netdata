// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupPlan(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out, diagnostic bytes.Buffer
	err := runRuntimeCommand(context.Background(), append([]string{"setup-plan"}, args...), nil, &out, &diagnostic)
	return out.String(), err
}

func TestRuntimeSetupPlanModesAndOverrides(t *testing.T) {
	for _, mode := range []string{"managed", "preloaded", "native"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "runtime")
			config := "runtime:\n  mode: " + mode + "\n"
			switch mode {
			case "managed":
				config += fmt.Sprintf("  cache_path: %q\n", path)
			case "preloaded":
				config += fmt.Sprintf("  preload_path: %q\n", path)
			case "native":
				path = runtime.DefaultNativeConfig().PayloadPath
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(config), 0600))
			canonical, err := runtime.CanonicalRoot(path)
			require.NoError(t, err)
			out, err := setupPlan(t, "--config-dir", dir)
			require.NoError(t, err)
			assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\n"+mode+"\n"+canonical+"\n", out)
			override := filepath.Join(dir, "other")
			canonicalOverride, err := runtime.CanonicalRoot(override)
			require.NoError(t, err)
			out, err = setupPlan(t, "--config-dir", dir, "--cache", override)
			require.NoError(t, err)
			if mode == "managed" {
				canonical = canonicalOverride
			}
			assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\n"+mode+"\n"+canonical+"\n", out)
			out, err = setupPlan(t, "--config-dir", dir, "--root", override)
			require.NoError(t, err)
			assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\npreloaded\n"+canonicalOverride+"\n", out)
			assert.NoDirExists(t, override)
		})
	}
}

func TestRuntimeSetupPlanUsesEffectiveOverride(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"),
		[]byte("runtime: {mode: managed, cache_path: relative}\n"), 0600))
	root := filepath.Join(dir, "chosen")
	canonical, err := runtime.CanonicalRoot(root)
	require.NoError(t, err)
	for _, test := range []struct{ flag, mode string }{{"--cache", "managed"}, {"--root", "preloaded"}} {
		t.Run(test.flag, func(t *testing.T) {
			out, err := setupPlan(t, "--config-dir", dir, test.flag, root)
			require.NoError(t, err)
			assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\n"+test.mode+"\n"+canonical+"\n", out)
			assert.NoDirExists(t, root)
		})
	}
}

func TestRuntimeSetupPlanDefaultsAndConfigPrecedence(t *testing.T) {
	oldUser, oldStock := buildinfo.UserConfigDir, buildinfo.StockConfigDir
	t.Cleanup(func() { buildinfo.UserConfigDir, buildinfo.StockConfigDir = oldUser, oldStock })
	user, stock, _, _, cli := runtimeConfigRoots(t)
	oldLib := buildinfo.VarLibDir
	t.Cleanup(func() { buildinfo.VarLibDir = oldLib })
	buildinfo.VarLibDir = filepath.Join(t.TempDir(), "build-lib")
	envLib := filepath.Join(t.TempDir(), "env-lib")
	t.Setenv("NETDATA_LIB_DIR", envLib)
	for _, tc := range []struct{ dir, leaf string }{{stock, "stock"}, {user, "user"}, {cli, "cli"}} {
		path := filepath.Join(t.TempDir(), tc.leaf)
		require.NoError(
			t,
			os.WriteFile(
				filepath.Join(tc.dir, "dem.conf"),
				[]byte(fmt.Sprintf("runtime: {cache_path: %q}\n", path)),
				0600,
			),
		)
	}
	for _, tc := range []struct {
		args []string
		leaf string
	}{{[]string{"--config-dir", cli}, "cli"}, {nil, "user"}} {
		out, err := setupPlan(t, tc.args...)
		require.NoError(t, err)
		assert.Contains(t, out, "/"+tc.leaf+"\n")
	}
	empty := t.TempDir()
	t.Setenv("NETDATA_USER_CONFIG_DIR", empty)
	out, err := setupPlan(t, "--config-dir", empty)
	require.NoError(t, err)
	assert.Contains(t, out, "/stock\n")
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", empty)
	for _, lib := range []string{envLib, buildinfo.VarLibDir} {
		if lib == buildinfo.VarLibDir {
			t.Setenv("NETDATA_LIB_DIR", "")
		}
		out, err := setupPlan(t, "--config-dir", empty)
		require.NoError(t, err)
		want, err := runtime.CanonicalRoot(filepath.Join(lib, "dem", "runtime"))
		require.NoError(t, err)
		assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n"+want+"\n", out)
		assert.NoDirExists(t, lib)
	}
}

func TestRuntimeSetupPlanRejectsInvalidPathsAndArguments(t *testing.T) {
	dir := t.TempDir()
	for _, path := range []string{"", "relative", "/tmp/new\nline", "/tmp/carriage\rreturn", "/tmp/tab\t", "/tmp/zero\x00", "/tmp/delete\x7f", "/" + strings.Repeat("x", 4096)} {
		for _, flag := range []string{"--config-dir", "--cache", "--root"} {
			out, err := setupPlan(t, "--config-dir", dir, flag, path)
			require.Error(t, err, "%s %q", flag, path)
			assert.Empty(t, out)
		}
		if path == "" {
			continue
		}
		for _, field := range []string{"cache_path", "preload_path"} {
			mode := "managed"
			if field == "preload_path" {
				mode = "preloaded"
			}
			require.NoError(
				t,
				os.WriteFile(
					filepath.Join(dir, "dem.conf"),
					[]byte(fmt.Sprintf("runtime: {mode: %s, %s: %q}\n", mode, field, path)),
					0600,
				),
			)
			out, err := setupPlan(t, "--config-dir", dir)
			require.Error(t, err)
			assert.Empty(t, out)
		}
	}
	for _, args := range [][]string{{"stray"}, {"--allow-download"}, {"--cache", dir, "--root", dir}, {"--archive", dir}} {
		out, err := setupPlan(t, args...)
		require.Error(t, err)
		assert.Empty(t, out)
	}
}

func TestRuntimeSetupPlanDoesNotAcquireOrMutate(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests.Add(1) }))
	defer server.Close()
	dir := t.TempDir()
	real, alias := filepath.Join(dir, "real"), filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(real, 0700))
	require.NoError(t, os.Symlink(real, alias))
	cache := filepath.Join(alias, "missing", "runtime")
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "dem.conf"),
			[]byte(fmt.Sprintf("runtime: {cache_path: %q, release_url: %q}\n", cache, server.URL)),
			0600,
		),
	)
	out, err := setupPlan(t, "--config-dir", dir)
	require.NoError(t, err)
	canonicalReal, err := filepath.EvalSymlinks(real)
	require.NoError(t, err)
	assert.Equal(
		t,
		"netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n"+filepath.Join(canonicalReal, "missing", "runtime")+"\n",
		out,
	)
	entries, err := os.ReadDir(real)
	require.NoError(t, err)
	assert.Empty(t, entries)
	assert.Zero(t, requests.Load())
	require.NoError(t, os.WriteFile(filepath.Join(real, "layout.json"), []byte("invalid"), 0600))
	_, err = setupPlan(t, "--config-dir", dir, "--cache", real)
	require.NoError(t, err)
	after, err := os.ReadFile(filepath.Join(real, "layout.json"))
	require.NoError(t, err)
	assert.Equal(t, "invalid", string(after))
	entries, err = os.ReadDir(real)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestRuntimeSetupPlanRejectsNonDirectoryRootsWithoutMutation(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte("runtime: {mode: managed}\n"), 0600))
	file, alias := filepath.Join(dir, "file"), filepath.Join(dir, "alias")
	require.NoError(t, os.WriteFile(file, []byte("preserved"), 0600))
	require.NoError(t, os.Symlink(file, alias))
	for _, path := range []string{file, alias, filepath.Join(file, "child"), filepath.Join(alias, "child")} {
		for _, flag := range []string{"--cache", "--root"} {
			out, err := setupPlan(t, "--config-dir", dir, flag, path)
			require.Error(t, err, "%s %s", flag, path)
			assert.Empty(t, out)
		}
	}
	data, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, "preserved", string(data))
	target, err := os.Readlink(alias)
	require.NoError(t, err)
	assert.Equal(t, file, target)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 3)
}

func TestRuntimeSetupPlanValidatesEnvironmentAndCanonicalPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NETDATA_USER_CONFIG_DIR", dir)
	t.Setenv("NETDATA_STOCK_CONFIG_DIR", dir)
	for _, path := range []string{"relative", "/tmp/bad\nroot", "/" + strings.Repeat("x", 4096)} {
		t.Setenv("NETDATA_LIB_DIR", path)
		out, err := setupPlan(t, "--config-dir", dir)
		require.Error(t, err)
		assert.Empty(t, out)
	}
	target, alias := filepath.Join(dir, "bad\nroot"), filepath.Join(dir, "alias")
	require.NoError(t, os.Mkdir(target, 0700))
	require.NoError(t, os.Symlink(target, alias))
	out, err := setupPlan(t, "--config-dir", dir, "--cache", alias)
	require.Error(t, err)
	assert.Empty(t, out)
	// Literal policy metacharacters remain data in the execution root protocol.
	literal := filepath.Join(dir, `space [glob]*?{brace}@"quote\slash`)
	out, err = setupPlan(t, "--config-dir", dir, "--cache", literal)
	require.NoError(t, err)
	canonical, err := runtime.CanonicalRoot(literal)
	require.NoError(t, err)
	assert.Equal(t, "netdata-dem-runtime-setup-v1\nstandalone\nmanaged\n"+canonical+"\n", out)
	assert.NoDirExists(t, literal)
}

func TestRuntimeSetupPlanBuildOwnershipCannotBeChangedByRuntimeConfig(t *testing.T) {
	previous := runtimehost.DefaultMode
	t.Cleanup(func() { runtimehost.DefaultMode = previous })
	runtimehost.DefaultMode = "native"
	dir := t.TempDir()
	root := filepath.Join(dir, "runtime")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(fmt.Sprintf("runtime: {mode: managed, cache_path: %q}\n", root)), 0600))
	for _, args := range [][]string{nil, {"--root", root}} {
		out, err := setupPlan(t, append([]string{"--config-dir", dir}, args...)...)
		require.NoError(t, err)
		assert.True(t, strings.HasPrefix(out, "netdata-dem-runtime-setup-v1\nnative-package\n"), out)
		assert.NoDirExists(t, root)
	}
}
