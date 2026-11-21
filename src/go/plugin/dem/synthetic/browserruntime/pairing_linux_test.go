// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/internal/browserruntimetest"
	"github.com/stretchr/testify/require"
)

func pairFixture(version string) AgentRelease {
	return AgentRelease{Version: version, Channel: "stable", Source: strings.Repeat("a", 40)}
}

func pairInfoFixture(agent AgentRelease) []byte {
	return []byte("version=" + agent.Version + "\nchannel=" + agent.Channel + "\nsource=" + agent.Source + "\n")
}

func pairedBundleFixture(t *testing.T, agent AgentRelease, payload []byte) (Release, string) {
	t.Helper()
	m, archive := bundleFixture(t, agent.Version, browserruntimetest.Entry{
		Name: releaseInfoName, Contents: payload, Mode: 0644,
	})
	m.Version = agent.Version
	release := publishedRelease(m, 0)
	release.Format, release.Agent = 2, &agent
	return release, archiveFile(t, archive)
}

func TestManagedExactPairKeepsPinnedOldConsumers(t *testing.T) {
	ctx := context.Background()
	first := pairFixture("v2.12.0")
	config := DefaultConfig(filepath.Join(t.TempDir(), "store"))
	config.Agent = &first
	oldStore, err := Open(config)
	require.NoError(t, err)
	defer oldStore.Close()
	previous, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, oldStore, previous, archive, ManualUpdates)
	old, err := oldStore.AcquireSelected()
	require.NoError(t, err)
	defer old.Close()

	second := pairFixture("v2.13.0")
	next, nextArchive := pairedBundleFixture(t, second, pairInfoFixture(second))
	_, err = oldStore.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: InstallRelease, Updates: ManualUpdates},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, next) })
	require.ErrorIs(t, err, ErrIncompatible, "runtime cannot independently advance past its Agent")
	status, err := oldStore.Inspect()
	require.NoError(t, err)
	require.Equal(t, previous, *status.Release)

	config.Agent = &second
	newStore, err := Open(config)
	require.NoError(t, err)
	defer newStore.Close()
	compatible, err := newStore.AcquireSelected()
	require.NoError(t, err, "execution compatibility is independent of installation pairing")
	require.NoError(t, compatible.Close())

	candidate, err := newStore.BeginPublishedInstall(ctx,
		ReleaseInstallOptions{Operation: InstallRelease, Updates: ManualUpdates},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, next) })
	if candidate != nil {
		require.NoError(t, candidate.Close())
	}
	require.ErrorContains(t, err, "installation owner", "a target helper alone cannot bypass paired preparation")
	candidate, err = newStore.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: PrepareRelease},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, next) })
	require.NoError(t, err)
	require.NoError(t, candidate.Import(ctx, nextArchive))
	require.ErrorContains(t, candidate.Select(), "paired preparation", "preparation must not publish selection")
	require.NoError(t, candidate.PreparePair())
	require.NoError(t, candidate.Close())
	use, err := old.BeginUse()
	require.NoError(t, err, "the old Agent's acquired pair remains pinned and usable")
	require.NoError(t, use.Finish(true))
	require.ErrorIs(t, newStore.ActivatePair(ctx), ErrInUse)
	require.NoError(t, old.Close())
	require.NoError(t, newStore.ActivatePair(ctx))
	fresh, err := newStore.AcquireSelected()
	require.NoError(t, err)
	require.Equal(t, next.Manifest, fresh.Manifest())
	require.NoError(t, fresh.Close())
	require.NoError(t, newStore.Uninstall(ctx))
}

func TestSignedPairMustMatchPreparedPayloadBeforeUseOrSelection(t *testing.T) {
	for _, mismatch := range []string{"version", "channel", "source", "malformed"} {
		t.Run(mismatch, func(t *testing.T) {
			agent := pairFixture("v2.12.0")
			wrong := agent
			switch mismatch {
			case "version":
				wrong.Version = "v2.11.0"
			case "channel":
				wrong.Channel = "nightly"
			case "source":
				wrong.Source = strings.Repeat("b", 40)
			}
			data := pairInfoFixture(wrong)
			if mismatch == "malformed" {
				data = []byte("not a release identity")
			}
			release, archive := pairedBundleFixture(t, agent, data)
			s, _ := testStore(t)
			candidate, err := s.beginRelease(t, context.Background(), release, InstallRelease)
			require.NoError(t, err)
			defer candidate.Close()
			require.NoError(t, candidate.Import(context.Background(), archive))
			_, err = candidate.BeginUse()
			require.ErrorIs(t, err, ErrDamaged)
			require.ErrorIs(t, candidate.Select(), ErrDamaged)
			require.ErrorIs(t, candidate.SealPreload(), ErrDamaged)
			_, err = s.AcquireSelected()
			require.ErrorIs(t, err, ErrUnavailable)
		})
	}
}

func TestPreloadChecksAgentPairWithoutChangingPersistedRecordShape(t *testing.T) {
	agent := pairFixture("v2.12.0")
	release, archive := pairedBundleFixture(t, agent, pairInfoFixture(agent))
	s, _ := testStore(t)
	candidate, err := s.beginRelease(t, context.Background(), release, InstallRelease)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.SealPreload())
	require.NoError(t, candidate.Close())
	_, actual, err := ReadPreload(s.path, &agent)
	require.NoError(t, err)
	require.Equal(t, release.Manifest, actual)
	_, actual, err = ReadPreload(s.path, nil)
	require.NoError(t, err, "execution validates provenance without the consumer's release stamp")
	require.Equal(t, release.Manifest, actual)
	wrong := agent
	wrong.Source = strings.Repeat("b", 40)
	_, _, err = ReadPreload(s.path, &wrong)
	require.ErrorIs(t, err, ErrIncompatible)
	require.NotErrorIs(t, err, errPreloadDamaged)
	record, err := os.ReadFile(filepath.Join(s.path, "preload.json"))
	require.NoError(t, err)
	require.NotContains(t, string(record), "\"agent\"", "old preload reader retains its existing record schema")
}

func TestUnchangedPreparationChecksRetainedPair(t *testing.T) {
	agent := pairFixture("v2.12.0")
	release, archive := pairedBundleFixture(t, agent, pairInfoFixture(agent))
	s, _ := testStore(t)
	installRelease(t, s, release, archive, ManualUpdates)
	path := filepath.Join(s.path, "generations", release.Manifest.Archive.SHA256, releaseInfoName)
	wrong := agent
	wrong.Source = strings.Repeat("b", 40)
	require.NoError(t, os.Chmod(path, 0600))
	require.NoError(t, os.WriteFile(path, pairInfoFixture(wrong), 0600))
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: PrepareRelease},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, release) })
	require.Nil(t, candidate)
	require.ErrorIs(t, err, ErrDamaged)
}

func TestNativePairChecksDoNotBlockClosingOldAdmission(t *testing.T) {
	f := newNativeFixture(t)
	agent := pairFixture(f.manifest.Version)
	require.NoError(t, os.WriteFile(filepath.Join(f.config.PayloadPath, releaseInfoName), pairInfoFixture(agent), 0644))
	f.config.Agent = &agent
	control, err := OpenNativeControl(f.config)
	require.NoError(t, err)
	ctx := context.Background()
	require.NoError(t, control.ValidateAndReady(ctx, f.inspect(t), f.validate(t, "validate")))
	data, err := f.command(t, "execute", nil).CombinedOutput()
	require.NoError(t, err, "%s", data)

	newAgent := pairFixture("next")
	for _, agent := range []AgentRelease{{}, newAgent, {Version: agent.Version, Channel: "nightly", Source: agent.Source},
		{Version: agent.Version, Channel: agent.Channel, Source: strings.Repeat("b", 40)}} {
		f.config.Agent = &agent
		data, err = f.command(t, "execute", nil).CombinedOutput()
		require.NoError(t, err, "%s", data)
		data, err = f.command(t, "pair-status", nil).CombinedOutput()
		require.Error(t, err, "package owner must still require the exact pair: %s", data)
	}
	f.config.Agent = &newAgent
	control, err = OpenNativeControl(f.config)
	require.NoError(t, err)
	require.NoError(t, control.CloseAdmission(ctx, f.inspect(t)), "a target helper can drain a healthy older pair")
	require.ErrorIs(t, control.ValidateAndReady(ctx, f.inspect(t), f.validate(t, "validate")), ErrIncompatible)
	state, err := readNativeControl(f.config)
	require.NoError(t, err)
	require.Equal(t, nativeClosed, state.Phase)
}

func TestFixedRuntimeExecutionRequiresIntactProvenance(t *testing.T) {
	for _, fault := range []string{"missing", "malformed", "version"} {
		t.Run("preload/"+fault, func(t *testing.T) {
			s, _ := testStore(t)
			m := importFixture(t, s, "preload")
			candidate, err := s.beginFixture(t, context.Background(), m, InstallRelease)
			require.NoError(t, err)
			require.NoError(t, candidate.SealPreload())
			require.NoError(t, candidate.Close())
			damagePayloadProvenance(t, filepath.Join(s.path, "generations", m.Archive.SHA256), fault)
			_, _, err = ReadPreload(s.path, nil)
			require.Error(t, err)
		})
		t.Run("native/"+fault, func(t *testing.T) {
			f := newNativeFixture(t)
			require.NoError(t, f.control.ValidateAndReady(context.Background(), f.inspect(t), f.validate(t, "validate")))
			damagePayloadProvenance(t, f.config.PayloadPath, fault)
			data, err := f.command(t, "execute", nil).CombinedOutput()
			require.Error(t, err, "%s", data)
		})
	}
}

func damagePayloadProvenance(t *testing.T, root, fault string) {
	t.Helper()
	path := filepath.Join(root, releaseInfoName)
	if fault == "missing" {
		require.NoError(t, os.Remove(path))
		return
	}
	data := []byte("invalid provenance")
	if fault == "version" {
		data = pairInfoFixture(pairFixture("wrong-version"))
	}
	require.NoError(t, os.Chmod(path, 0644))
	require.NoError(t, os.WriteFile(path, data, 0644))
}
