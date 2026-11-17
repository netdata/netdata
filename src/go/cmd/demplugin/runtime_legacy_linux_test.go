// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/internal/pgptest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/stretchr/testify/require"
)

// Supply an unmodified previous dem.plugin binary to exercise its real strict
// metadata reader and process-owned GC, rather than simulating an older identity.
func TestPreviousBinaryReadsRollbackAndLegacyGCAllowsRetainedInputRetry(t *testing.T) {
	legacy := os.Getenv("DEM_RUNTIME_LEGACY_CLI")
	if legacy == "" {
		t.Skip("requires an unmodified previous dem.plugin binary")
	}
	require.True(t, filepath.IsAbs(legacy))
	config := t.TempDir()
	cache := filepath.Join(config, "runtime")
	writeAdminTrust(t, config, cache)
	releasePath, archive := adminBundleFixture(t)
	verified, err := browserruntime.ReadRelease(releasePath, adminTrust(t))
	require.NoError(t, err)
	release := verified.Release()
	release.Format, release.Sequence, release.Agent = 1, 1, nil
	envelope, err := pgptest.Release(release, adminSigningKey(t))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(releasePath, envelope, 0600))
	verified, err = browserruntime.ReadRelease(releasePath, adminTrust(t))
	require.NoError(t, err)
	// Seed an authenticated old-schema installation through the store, because
	// this fixture cannot pass the previous CLI's mandatory browser smoke test.
	store, err := browserruntime.Open(browserruntime.DefaultConfig(cache))
	require.NoError(t, err)
	candidate, err := store.BeginPublishedInstall(context.Background(), browserruntime.ReleaseInstallOptions{
		Operation: browserruntime.InstallRelease, Updates: browserruntime.ManualUpdates,
	}, func(context.Context) (browserruntime.VerifiedRelease, error) { return verified, nil })
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	require.NoError(t, store.Close())
	selectionPath := filepath.Join(cache, "selection.json")
	original, err := os.ReadFile(selectionPath)
	require.NoError(t, err)
	oldStatus := func() {
		t.Helper()
		command := exec.Command(legacy, "runtime", "status", "--config-dir", config, "--cache", cache)
		data, err := command.CombinedOutput()
		require.NoError(t, err, "%s", data)
	}
	oldStatus()

	setRuntimeAgent(t, fixtureAgent("paired-target"))
	releasePath, archive = adminNamedBundleFixture(t, "paired-target")
	report, err := runtimeCommand(t, "runtime", "fetch", "--config-dir", config,
		"--release", releasePath, "--archive", archive, "--output", filepath.Join(config, "retained-inputs"))
	require.NoError(t, err)
	inputs := report.Inputs
	prepare := func() runtimeCommandReport {
		t.Helper()
		report, err := runtimeCommand(t, "runtime", "prepare", "--config-dir", config, "--cache", cache,
			"--release", inputs.Release, "--archive", inputs.Archive, "--keyring", inputs.Keyring)
		require.NoError(t, err)
		return report
	}
	report = prepare()
	target := filepath.Join(cache, "generations", report.Candidate.SHA256)
	require.DirExists(t, target)
	oldStatus()

	// Simulate an interrupted first migration followed by restarting the old Agent.
	// Its startup sweep does not know the new recovery record and retires the target.
	writeRuntimeTrustConfig(t, config, cache, filepath.Join(config, "release-public.gpg"))
	configuration, err := os.OpenFile(filepath.Join(config, "dem.conf"), os.O_APPEND|os.O_WRONLY, 0600)
	require.NoError(t, err)
	_, err = configuration.WriteString("  previous_retention_days: 0\ndefault_run: false\nmodules:\n  receiver: false\n  rum: false\n  journey: false\n  lighthouse: false\n")
	require.NoError(t, err)
	require.NoError(t, configuration.Close())
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, legacy, "-c", config)
	command.Env = append(os.Environ(), "NETDATA_LIB_DIR="+filepath.Join(config, "history"),
		"NETDATA_STOCK_CONFIG_DIR="+config, "NETDATA_USER_CONFIG_DIR="+config)
	var log bytes.Buffer
	command.Stdout, command.Stderr = &log, &log
	stdin, err := command.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, command.Start())
	t.Cleanup(func() { cancel(); _ = stdin.Close() })
	removed := false
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(target); os.IsNotExist(err) {
			removed = true
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	_ = command.Process.Signal(syscall.SIGTERM)
	err = command.Wait()
	_ = stdin.Close()
	require.NoError(t, err, "%s", log.String())
	require.True(t, removed, "previous binary must actually run its sweep: %s", log.String())
	require.FileExists(t, inputs.Archive)
	prepare()
	require.DirExists(t, target, "retained authenticated inputs reimport the generation removed by old GC")
	_, err = runtimeCommand(t, "runtime", "activate", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
	_, err = runtimeCommand(t, "runtime", "rollback", "--config-dir", config, "--cache", cache)
	require.NoError(t, err)
	restored, err := os.ReadFile(selectionPath)
	require.NoError(t, err)
	require.Equal(t, original, restored)
	oldStatus()
}
