// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/pluginconfig"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtimehost"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFetchPrepareActivateAndRollback(t *testing.T) {
	release, archive := adminBundleFixture(t)
	config := t.TempDir()
	writeAdminTrust(t, config, "")
	cache := filepath.Join(config, "cache")
	_, err := runtimeCommand(t, "runtime", "install", "--config-dir", config, "--cache", cache,
		"--release", release, "--archive", archive, "--manual-updates")
	require.NoError(t, err)
	selection := filepath.Join(cache, "selection.json")
	before, err := os.ReadFile(selection)
	require.NoError(t, err)
	target := fixtureAgent("target-agent")
	setRuntimeAgent(t, target)
	release, archive = adminNamedBundleFixture(t, target.Version)
	inputs := filepath.Join(config, "retained-inputs")
	report, err := runtimeCommand(t, "runtime", "fetch", "--config-dir", config, "--output", inputs,
		"--release", release, "--archive", archive)
	require.NoError(t, err)
	require.Equal(t, "inputs-verified", report.State)
	require.NotNil(t, report.Inputs)
	require.Nil(t, report.Qualification)
	after, err := os.ReadFile(selection)
	require.NoError(t, err)
	require.Equal(t, before, after, "fetch does not touch selection")
	// An online installation need not have a configured offline keyring. Its
	// owner explicitly supplies only the trust retained by the authenticated fetch.
	require.NoError(t, os.WriteFile(filepath.Join(config, "dem.conf"), []byte("runtime: {}\n"), 0600))
	report, err = runtimeCommand(t, "runtime", "prepare", "--config-dir", config, "--cache", cache,
		"--release", report.Inputs.Release, "--archive", report.Inputs.Archive, "--keyring", report.Inputs.Keyring)
	require.NoError(t, err)
	require.Equal(t, "prepared", report.Outcome)
	require.Equal(t, "prepared", report.Storage.Recovery.State)
	require.False(t, report.Storage.AutomaticUpdates)
	require.Nil(t, report.Qualification)
	after, err = os.ReadFile(selection)
	require.NoError(t, err)
	require.Equal(t, before, after, "prepare leaves the old Agent selection intact")
	report, err = runtimeCommand(t, "runtime", "activate", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
	require.Equal(t, target.Version, report.Runtime.Version)
	require.Equal(t, "rollback-available", report.Storage.Recovery.State)
	report, err = runtimeCommand(t, "runtime", "rollback", "--prepared-only", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
	require.Equal(t, target.Version, report.Runtime.Version, "cancel with no pending record must not roll back the previous pair")
	report, err = runtimeCommand(t, "runtime", "rollback", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
	require.Equal(t, "cli-fixture", report.Runtime.Version)
	require.Nil(t, report.Qualification)
	after, err = os.ReadFile(selection)
	require.NoError(t, err)
	require.Equal(t, before, after, "rollback restores the exact old selection schema and consent")
	setRuntimeAgent(t, fixtureAgent("cli-fixture"))
	_, err = runtimeCommand(t, "runtime", "status", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
}

func TestRuntimePairPlanReadsEnrollmentWithoutInitializingStorage(t *testing.T) {
	for _, state := range []string{"absent", "empty", "manual", "automatic", "corrupt"} {
		t.Run(state, func(t *testing.T) {
			config := t.TempDir()
			cache := filepath.Join(config, "runtime")
			writeAdminTrust(t, config, cache)
			if state == "empty" {
				require.NoError(t, os.Mkdir(cache, 0700))
			} else if state != "absent" {
				release, archive := adminBundleFixture(t)
				args := []string{"runtime", "install", "--config-dir", config, "--release", release, "--archive", archive}
				if state != "automatic" {
					args = append(args, "--manual-updates")
				}
				_, err := runtimeCommand(t, args...)
				require.NoError(t, err)
				if state == "corrupt" {
					require.NoError(t, os.WriteFile(filepath.Join(cache, "recovery.json"), []byte("broken"), 0600))
				}
				setRuntimeAgent(t, fixtureAgent("new-agent"))
			}
			result := runtimeRun(t, context.Background(), "runtime", "pair-plan", "--config-dir", config)
			if state == "empty" || state == "corrupt" {
				require.Error(t, result.err)
				require.Empty(t, result.stdout)
				if state == "empty" {
					entries, err := os.ReadDir(cache)
					require.NoError(t, err)
					require.Empty(t, entries, "planning must not complete or create initialization")
				}
				return
			}
			require.NoError(t, result.err)
			enrollment, consent := "enrolled", "manual"
			if state == "absent" {
				enrollment = "absent"
				require.NoDirExists(t, cache)
			} else if state == "automatic" {
				consent = "automatic"
			}
			require.Equal(t, []string{"netdata-dem-pair-plan-v1", "managed", enrollment, consent, cache, ""},
				strings.Split(result.stdout, "\n"))
		})
	}
}

func TestRuntimePairPlanNamesTheConfiguredCanonicalPreload(t *testing.T) {
	config := t.TempDir()
	root := filepath.Join(config, "sealed-runtime")
	writeAdminTrust(t, config, "")
	release, archive := adminBundleFixture(t)
	_, err := runtimeCommand(t, "runtime", "preload", "--config-dir", config,
		"--root", root, "--release", release, "--archive", archive)
	require.NoError(t, err)
	alias := filepath.Join(config, "preload-alias")
	require.NoError(t, os.Symlink(root, alias))
	require.NoError(t, os.WriteFile(filepath.Join(config, "dem.conf"),
		[]byte("runtime:\n  mode: preloaded\n  preload_path: "+alias+"\n"), 0600))
	result := runtimeRun(t, context.Background(), "runtime", "pair-plan", "--config-dir", config)
	require.NoError(t, result.err)
	require.Equal(t, "netdata-dem-pair-plan-v1\npreloaded\nenrolled\nmanual\n"+root+"\n", result.stdout)
	_, err = runtimeCommand(t, "runtime", "preload-validate", "--config-dir", config)
	require.NoError(t, err, "the owner can validate the target preload separately without browser execution")
}

func TestRuntimeExecutionDoesNotRequireMatchingBuildIdentity(t *testing.T) {
	for _, mode := range []string{"managed", "preloaded"} {
		for _, stamp := range []string{"unstamped", "version", "channel", "source"} {
			t.Run(mode+"/"+stamp, func(t *testing.T) {
				config := t.TempDir()
				root := filepath.Join(config, "runtime")
				writeAdminTrust(t, config, root)
				release, archive := adminBundleFixture(t)
				command := []string{"runtime", "install", "--config-dir", config, "--release", release, "--archive", archive}
				if mode == "preloaded" {
					command[1] = "preload"
					command = append(command, "--root", root)
				}
				_, err := runtimeCommand(t, command...)
				require.NoError(t, err)
				if mode == "preloaded" {
					require.NoError(t, os.WriteFile(filepath.Join(config, "dem.conf"),
						[]byte("runtime:\n  mode: preloaded\n  preload_path: "+root+"\n"), 0600))
				}
				agent := runtimehost.AgentRelease()
				switch stamp {
				case "unstamped":
					agent = browserruntime.AgentRelease{}
				case "version":
					agent.Version = "v99.0.0"
				case "channel":
					agent.Channel = "nightly"
				case "source":
					agent.Source = strings.Repeat("b", 40)
				}
				setRuntimeAgent(t, agent)
				report, err := runtimeCommand(t, "runtime", "status", "--config-dir", config)
				require.NoError(t, err)
				require.Equal(t, "installed", report.State)
				cfg, err := dem.LoadConfig(pluginconfig.ResolveConfigDirs([]string{config}))
				require.NoError(t, err)
				handle := runtimehost.Open(cfg.Runtime, "")
				t.Cleanup(func() { require.NoError(t, handle.Close()) })
				require.NoError(t, handle.SetupError)
				use, err := handle.Source.BeginUse()
				require.NoError(t, err)
				require.NoError(t, use.Finish(true))
				if mode == "preloaded" {
					_, err = runtimeCommand(t, "runtime", "preload-validate", "--config-dir", config)
					require.Error(t, err, "installation owners still require the exact pair")
					if stamp != "unstamped" {
						require.ErrorIs(t, err, browserruntime.ErrIncompatible)
					}
				}
			})
		}
	}
}

func TestRuntimeSourceUpdatePlan(t *testing.T) {
	for name, test := range map[string]struct {
		agent     browserruntime.AgentRelease
		want      string
		wantError string
	}{
		"unstamped": {want: "retain\n"},
		"published": {agent: fixtureAgent("next-release"), want: "paired\n"},
		"partial stamp": {agent: browserruntime.AgentRelease{
			Version: "partial",
		}, wantError: "expected Agent release"},
	} {
		t.Run(name, func(t *testing.T) {
			config := t.TempDir()
			cache := filepath.Join(config, "runtime")
			writeAdminTrust(t, config, cache)
			release, archive := adminBundleFixture(t)
			_, err := runtimeCommand(t, "runtime", "install", "--config-dir", config,
				"--release", release, "--archive", archive, "--manual-updates")
			require.NoError(t, err)
			before, err := os.ReadFile(filepath.Join(cache, "selection.json"))
			require.NoError(t, err)
			setRuntimeAgent(t, test.agent)
			result := runtimeRun(
				t,
				context.Background(),
				"runtime",
				"pair-plan",
				"--source-update",
				"--config-dir",
				config,
			)
			if test.wantError != "" {
				require.ErrorContains(t, result.err, test.wantError)
			} else {
				require.NoError(t, result.err)
			}
			require.Equal(t, test.want, result.stdout)
			after, err := os.ReadFile(filepath.Join(cache, "selection.json"))
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestRuntimeSourceUpdatePlanValidatesPreloadWithoutExactPair(t *testing.T) {
	config := t.TempDir()
	writeAdminTrust(t, config, "")
	release, archive := adminBundleFixture(t)
	root := filepath.Join(config, "preload")
	_, err := runtimeCommand(t, "runtime", "preload", "--config-dir", config,
		"--root", root, "--release", release, "--archive", archive)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(config, "dem.conf"),
		[]byte("runtime:\n  mode: preloaded\n  preload_path: "+root+"\n"), 0600))
	setRuntimeAgent(t, browserruntime.AgentRelease{})
	result := runtimeRun(t, context.Background(), "runtime", "pair-plan", "--source-update", "--config-dir", config)
	require.NoError(t, result.err)
	require.Equal(t, "retain\n", result.stdout)
}

func TestRuntimeFetchDoesNotEnrollAndRejectsAlteredInputs(t *testing.T) {
	for _, fault := range []string{"none", "archive", "version", "existing-output"} {
		t.Run(fault, func(t *testing.T) {
			release, archive := adminBundleFixture(t)
			config := t.TempDir()
			cache := filepath.Join(config, "absent-runtime")
			writeAdminTrust(t, config, cache)
			output := filepath.Join(config, "inputs")
			switch fault {
			case "archive":
				require.NoError(t, os.WriteFile(archive, []byte("damaged bytes"), 0600))
			case "version":
				setRuntimeAgent(t, fixtureAgent("other"))
			case "existing-output":
				require.NoError(t, os.Mkdir(output, 0700))
				require.NoError(t, os.WriteFile(filepath.Join(output, "keep"), []byte("operator file"), 0600))
			}
			report, err := runtimeCommand(t, "runtime", "fetch", "--config-dir", config, "--output", output,
				"--release", release, "--archive", archive)
			if fault == "none" {
				require.NoError(t, err)
				require.FileExists(t, report.Inputs.Release)
				require.FileExists(t, report.Inputs.Archive)
				require.FileExists(t, report.Inputs.Keyring)
			} else {
				require.Error(t, err)
				require.Nil(t, report.Inputs)
				if fault == "version" {
					require.ErrorIs(t, err, browserruntime.ErrIncompatible)
				}
				if fault == "existing-output" {
					data, err := os.ReadFile(filepath.Join(output, "keep"))
					require.NoError(t, err)
					require.Equal(t, "operator file", string(data))
				} else {
					require.NoDirExists(t, output)
				}
			}
			require.NoDirExists(t, cache)
			require.Nil(t, report.Qualification)
		})
	}
}
