// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runtimeCommand(t *testing.T, args ...string) (runtimeCommandReport, error) {
	t.Helper()
	result := runtimeRun(t, context.Background(), args...)
	var report runtimeCommandReport
	if result.stdout != "" {
		require.NoError(t, json.Unmarshal([]byte(result.stdout), &report), result.stdout)
	}
	return report, result.err
}

type runtimeResult struct {
	stdout, stderr string
	err            error
}

// runtimeRun runs a `runtime ...` command line without a stdin control channel
// and returns its raw output streams.
func runtimeRun(t *testing.T, ctx context.Context, args ...string) runtimeResult {
	t.Helper()
	require.Equal(t, "runtime", args[0])
	var out, diagnostic bytes.Buffer
	err := runRuntimeCommand(ctx, args[1:], nil, &out, &diagnostic)
	return runtimeResult{stdout: out.String(), stderr: diagnostic.String(), err: err}
}

// jsonText renders a string as a JSON literal for golden reports.
func jsonText(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

const runtimeFlagUsage = `  -allow-download
    	explicitly download canonical package-signing keys, signed release metadata and runtime bytes; install/repair enables automatic preparation unless --manual-updates
  -archive string
    	offline archive checked against the trusted release descriptor
  -cache string
    	managed installation directory
  -cancel-on-stdin-close
    	cancel ensure cooperatively when its private stdin control channel closes
  -config-dir string
    	directory containing administrator dem.conf
  -expected-root string
    	require this canonical managed root before ensure mutates storage
  -manual-updates
    	disable automatic daily update preparation (install/repair); activation always requires restart
  -release string
    	local signed release descriptor; otherwise use configured latest URL
  -root string
    	preload final root (must be new or empty; never move it)
`

func TestRuntimeCommandHelpAndFlagErrors(t *testing.T) {
	for _, command := range []string{"ensure", "install", "update", "repair", "uninstall", "preload", "status", "qualify"} {
		t.Run(command, func(t *testing.T) {
			usage := "Usage of runtime " + command + ":\n" + runtimeFlagUsage
			result := runtimeRun(t, context.Background(), "runtime", command, "-h")
			assert.Equal(t, runtimeResult{stderr: usage}, result)
			result = runtimeRun(t, context.Background(), "runtime", command, "--bogus")
			assert.Equal(t, "flag provided but not defined: -bogus\n"+usage, result.stderr)
			assert.EqualError(t, result.err, "flag provided but not defined: -bogus")
			assert.Empty(t, result.stdout)
		})
	}
	result := runtimeRun(t, context.Background(), "runtime", "setup-plan", "-h")
	assert.Equal(t, runtimeResult{stderr: `Usage of runtime setup-plan:
  -cache string
    	managed installation directory
  -config-dir string
    	directory containing administrator dem.conf
  -root string
    	preload final root
`}, result)
}

// Argument errors precede configuration loading: they print no report, and the
// first failing rule wins in the order the cases list for combined mistakes.
func TestRuntimeCommandArgumentErrors(t *testing.T) {
	invalid := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(invalid, "dem.conf"), []byte("runtime: {mode: bogus}\n"), 0600))
	for name, tc := range map[string]struct {
		args    []string
		message string
	}{
		"no command":                {[]string{"runtime"}, "usage: dem.plugin runtime {ensure|install|update|repair|uninstall|preload|status|qualify|setup-plan} [flags]"},
		"unknown command":           {[]string{"runtime", "import"}, `unknown runtime command "import"`},
		"unknown native command":    {[]string{"runtime", "native-bogus", "--x"}, `unknown internal native runtime command "native-bogus"`},
		"native arguments":          {[]string{"runtime", "native-inspect", "stray"}, "internal native runtime commands accept no flags or positional arguments"},
		"setup-plan positional":     {[]string{"runtime", "setup-plan", "stray"}, "unexpected positional setup-plan arguments"},
		"positional":                {[]string{"runtime", "status", "stray"}, "unexpected positional runtime arguments"},
		"expected root not ensure":  {[]string{"runtime", "install", "--expected-root", "relative"}, "--expected-root and --cancel-on-stdin-close are only valid for ensure"},
		"cancel not ensure":         {[]string{"runtime", "status", "--cancel-on-stdin-close"}, "--expected-root and --cancel-on-stdin-close are only valid for ensure"},
		"expected root relative":    {[]string{"runtime", "ensure", "--expected-root", "relative", "--config-dir", "relative"}, "setup-plan paths must be absolute and at most 4096 bytes"},
		"cancel without channel":    {[]string{"runtime", "ensure", "--cancel-on-stdin-close", "--config-dir", "relative"}, "--cancel-on-stdin-close requires a private stdin control channel"},
		"config dir relative":       {[]string{"runtime", "status", "--config-dir", "relative", "--allow-download"}, "--config-dir must be absolute"},
		"release relative":          {[]string{"runtime", "install", "--release", "relative"}, "--release must be absolute"},
		"no source":                 {[]string{"runtime", "update"}, "provide either --archive or --allow-download for ensure/install/update/repair/preload"},
		"two sources":               {[]string{"runtime", "preload", "--release", "/r", "--archive", "/a", "--allow-download"}, "provide either --archive or --allow-download for ensure/install/update/repair/preload"},
		"archive without release":   {[]string{"runtime", "repair", "--archive", "relative"}, "offline archive requires an absolute --release path"},
		"source for status":         {[]string{"runtime", "status", "--release", "/r", "--manual-updates"}, "--release, --archive and --allow-download are only valid for ensure/install/update/repair/preload"},
		"source for uninstall":      {[]string{"runtime", "uninstall", "--archive", "/a"}, "--release, --archive and --allow-download are only valid for ensure/install/update/repair/preload"},
		"source for qualify":        {[]string{"runtime", "qualify", "--allow-download"}, "--release, --archive and --allow-download are only valid for ensure/install/update/repair/preload"},
		"manual updates for update": {[]string{"runtime", "update", "--manual-updates", "--allow-download", "--root", "/p"}, "--manual-updates is only valid for install/repair; update preserves the installed policy"},
		"archive relative":          {[]string{"runtime", "install", "--release", "/r", "--archive", "relative", "--root", "/p"}, "--archive must be absolute"},
		"preload without root":      {[]string{"runtime", "preload", "--allow-download"}, "preload requires an absolute --root final directory"},
		"preload relative root":     {[]string{"runtime", "preload", "--allow-download", "--root", "relative", "--cache", "/c"}, "preload requires an absolute --root final directory"},
		"preload with cache":        {[]string{"runtime", "preload", "--allow-download", "--root", "/p", "--cache", "/c"}, "preload uses --root, not --cache"},
		"root for install":          {[]string{"runtime", "install", "--allow-download", "--root", "/p"}, "--root is only valid for preload"},
		"invalid configuration":     {[]string{"runtime", "status", "--config-dir", invalid}, "runtime.mode must be managed, native or preloaded"},
	} {
		t.Run(name, func(t *testing.T) {
			result := runtimeRun(t, context.Background(), tc.args...)
			assert.EqualError(t, result.err, tc.message)
			assert.Empty(t, result.stdout)
			assert.Empty(t, result.stderr)
		})
	}
}

// Once configuration loads, every outcome prints one report, including refusals.
func TestRuntimeCommandReportsRefusals(t *testing.T) {
	root := t.TempDir()
	preloaded, native := filepath.Join(root, "preloaded"), filepath.Join(root, "native")
	for dir, conf := range map[string]string{preloaded: "runtime: {mode: preloaded, preload_path: /absent/bundle}\n", native: "runtime: {mode: native}\n"} {
		require.NoError(t, os.Mkdir(dir, 0700))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "dem.conf"), []byte(conf), 0600))
	}
	absent := filepath.Join(root, "absent")
	missing := jsonText(t, "runtime installation: stat "+absent+": no such file or directory")
	for name, tc := range map[string]struct {
		args   []string
		stdout string
		err    string
	}{
		"preloaded install": {
			args: []string{"runtime", "install", "--config-dir", preloaded, "--release", "/r.json", "--archive", "/a"},
			stdout: `{
  "download_allowed": false,
  "command": "install",
  "mode": "preloaded",
  "state": "unavailable",
  "release_path": "/r.json",
  "setup_error": "install/update/repair/uninstall require runtime.mode: managed"
}
`,
			err: "install/update/repair/uninstall require runtime.mode: managed",
		},
		"native repair": {
			args: []string{"runtime", "repair", "--config-dir", native, "--allow-download"},
			stdout: `{
  "download_allowed": true,
  "command": "repair",
  "mode": "native",
  "state": "unknown",
  "setup_error": "native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package"
}
`,
			err: "native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package",
		},
		"native ensure": {
			args: []string{"runtime", "ensure", "--config-dir", native, "--allow-download"},
			stdout: `{
  "outcome": "failed",
  "download_allowed": true,
  "command": "ensure",
  "mode": "native",
  "state": "unavailable",
  "setup_error": "native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package"
}
`,
			err: "native runtime installation, update, repair and removal belong to the package manager; install the optional netdata-dem-runtime package",
		},
		"absent status": {
			args: []string{"runtime", "status", "--config-dir", root, "--cache", absent},
			stdout: `{
  "download_allowed": false,
  "command": "status",
  "mode": "managed",
  "state": "unavailable",
  "cache_path": ` + jsonText(t, absent) + `,
  "setup_error": ` + missing + `
}
`,
			err: "runtime installation: stat " + absent + ": no such file or directory",
		},
		"absent qualify": {
			args: []string{"runtime", "qualify", "--config-dir", root, "--cache", absent},
			stdout: `{
  "download_allowed": false,
  "command": "qualify",
  "mode": "managed",
  "state": "unavailable",
  "cache_path": ` + jsonText(t, absent) + `,
  "setup_error": ` + missing + `
}
`,
			err: "runtime installation: stat " + absent + ": no such file or directory",
		},
		"absent uninstall": {
			args: []string{"runtime", "uninstall", "--config-dir", root, "--cache", absent},
			stdout: `{
  "download_allowed": false,
  "command": "uninstall",
  "mode": "managed",
  "state": "unavailable",
  "cache_path": ` + jsonText(t, absent) + `,
  "setup_error": ` + missing + `
}
`,
			err: "runtime installation: stat " + absent + ": no such file or directory",
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := runtimeRun(t, context.Background(), tc.args...)
			assert.Equal(t, tc.stdout, result.stdout)
			assert.EqualError(t, result.err, tc.err)
			assert.Empty(t, result.stderr)
		})
	}
	// Fixed modes report the runtime host's own setup failure for status and qualify.
	for _, mode := range []string{"preloaded", "native"} {
		dir := filepath.Join(root, mode)
		config, err := dem.LoadConfig([]string{dir})
		require.NoError(t, err)
		handle := runtimehost.Open(config.Runtime, "")
		require.Error(t, handle.SetupError)
		setupError := handle.SetupError.Error()
		require.NoError(t, handle.Close())
		for _, command := range []string{"status", "qualify"} {
			t.Run(mode+" "+command, func(t *testing.T) {
				result := runtimeRun(t, context.Background(), "runtime", command, "--config-dir", dir)
				assert.Equal(t, `{
  "download_allowed": false,
  "command": "`+command+`",
  "mode": "`+mode+`",
  "state": "unavailable",
  "setup_error": `+jsonText(t, setupError)+`
}
`, result.stdout)
				assert.EqualError(t, result.err, setupError)
				assert.Empty(t, result.stderr)
			})
		}
	}
}

func TestRuntimeEnsureReportsRefusalsBeforeStorage(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ensure requires the nonroot service identity")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "absent", "cache")
	canonical, err := runtime.CanonicalRoot(cache)
	require.NoError(t, err)
	mismatch := fmt.Sprintf("configured canonical runtime root %q differs from --expected-root %q; no runtime storage changed", canonical, "/var/lib/netdata/dem/runtime")
	result := runtimeRun(t, context.Background(), "runtime", "ensure", "--allow-download", "--config-dir", root, "--cache", cache, "--expected-root", "/var/lib/netdata/dem/runtime")
	assert.Equal(t, `{
  "outcome": "failed",
  "download_allowed": true,
  "command": "ensure",
  "mode": "managed",
  "state": "unavailable",
  "setup_error": `+jsonText(t, mismatch)+`
}
`, result.stdout)
	assert.EqualError(t, result.err, mismatch)
	assert.Empty(t, result.stderr)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result = runtimeRun(t, ctx, "runtime", "ensure", "--config-dir", root, "--cache", cache, "--release", "/r.json", "--archive", "/a")
	assert.Equal(t, `{
  "outcome": "failed",
  "download_allowed": false,
  "command": "ensure",
  "mode": "managed",
  "state": "unavailable",
  "release_path": "/r.json",
  "cache_path": `+jsonText(t, canonical)+`,
  "setup_error": "context canceled"
}
`, result.stdout)
	assert.ErrorIs(t, result.err, context.Canceled)
	assert.Empty(t, result.stderr)
	assert.NoDirExists(t, filepath.Dir(cache))
}
func TestRuntimeDispatchAndBadArguments(t *testing.T) {
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
