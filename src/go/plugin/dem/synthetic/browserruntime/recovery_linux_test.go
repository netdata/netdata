// SPDX-License-Identifier: GPL-3.0-or-later

package browserruntime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func openPairFixture(t *testing.T, path string, agent AgentRelease) *Store {
	t.Helper()
	config := DefaultConfig(path)
	config.Agent = &agent
	config.PreviousRetention = 0
	s, err := Open(config)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}

func preparePairFixture(t *testing.T, s *Store, release Release, archive string) {
	t.Helper()
	candidate, err := s.BeginPublishedInstall(context.Background(), ReleaseInstallOptions{Operation: PrepareRelease},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, release) })
	require.NoError(t, err)
	require.NotNil(t, candidate)
	require.NoError(t, candidate.Import(context.Background(), archive))
	require.NoError(t, candidate.PreparePair())
	require.NoError(t, candidate.Close())
}

func TestPairPreparationSurvivesHelperExitAndRequiresDrainedActivation(t *testing.T) {
	ctx := context.Background()
	first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
	previous, oldArchive := pairedBundleFixture(t, first, pairInfoFixture(first))
	target, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	root := filepath.Join(t.TempDir(), "runtime")
	old := openPairFixture(t, root, first)
	installRelease(t, old, previous, oldArchive, ManualUpdates)
	consumer, err := old.AcquireSelected()
	require.NoError(t, err)
	defer consumer.Close()
	s := openPairFixture(t, root, second)
	preparePairFixture(t, s, target, archive)
	preparePairFixture(t, s, target, archive)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, previous, *status.Release)
	require.Equal(t, "prepared", status.Recovery.State)
	require.Equal(t, target, *status.Recovery.Prepared)
	require.False(t, status.AutomaticUpdates, "preparation preserves installed consent")
	require.NoError(t, s.Close())
	s = openPairFixture(t, root, second)
	require.NoError(t, s.Sweep())
	require.DirExists(t, filepath.Join(root, "generations", target.Manifest.Archive.SHA256))
	require.ErrorIs(t, s.ActivatePair(ctx), ErrInUse)
	use, err := consumer.BeginUse()
	require.NoError(t, err, "preparation does not alter the old running pair")
	require.NoError(t, use.Finish(true))
	require.NoError(t, consumer.Close())
	require.NoError(t, s.ActivatePair(ctx))
	require.NoError(t, s.ActivatePair(ctx), "activation retry is harmless")
	require.NoError(t, s.Sweep())
	require.DirExists(t, filepath.Join(root, "generations", previous.Manifest.Archive.SHA256))
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, target, *status.Release)
	require.Equal(t, "rollback-available", status.Recovery.State)
	require.Equal(t, previous, *status.Recovery.Previous)
	require.NoError(t, s.RollbackPair(ctx))
	require.NoError(t, s.RollbackPair(ctx), "rollback retry does not toggle back to the new pair")
	consumer, err = old.AcquireSelected()
	require.NoError(t, err)
	require.NoError(t, consumer.Close())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, previous, *status.Release)
	require.Equal(t, "restored", status.Recovery.State)
}

func TestPendingPairReplacesOlderRollbackWithCurrentPair(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runtime")
	first := pairFixture("v2.12.0")
	s := openPairFixture(t, root, first)
	a, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, a, archive, AutomaticUpdates)
	second := pairFixture("v2.13.0")
	s = openPairFixture(t, root, second)
	b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, b, archive)
	require.NoError(t, s.ActivatePair(ctx))
	third := pairFixture("v2.14.0")
	s = openPairFixture(t, root, third)
	c, archive := pairedBundleFixture(t, third, pairInfoFixture(third))
	preparePairFixture(t, s, c, archive)
	require.NoError(t, s.Sweep())
	require.NoDirExists(t, filepath.Join(root, "generations", a.Manifest.Archive.SHA256))
	for _, release := range []Release{b, c} {
		require.DirExists(t, filepath.Join(root, "generations", release.Manifest.Archive.SHA256))
	}
	require.ErrorIs(t, s.Uninstall(ctx), ErrPairPending)
	_, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: RepairRelease, Updates: ManualUpdates},
		func(context.Context) (VerifiedRelease, error) {
			t.Fatal("pending repair must not fetch")
			return VerifiedRelease{}, nil
		})
	require.ErrorIs(t, err, ErrPairPending)
	require.NoError(t, s.RollbackPair(ctx), "canceling preparation retains this attempt's recovery point")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, b, *status.Release)
	require.Equal(t, b, *status.Recovery.Previous)
	preparePairFixture(t, s, c, archive)
	require.NoError(t, s.ActivatePair(ctx))
	require.NoError(t, s.Sweep())
	require.NoDirExists(t, filepath.Join(root, "generations", a.Manifest.Archive.SHA256))
	require.DirExists(t, filepath.Join(root, "generations", b.Manifest.Archive.SHA256))
}

func TestPendingRollbackRetryDoesNotStepBackAnotherPair(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runtime")
	first, second, third := pairFixture("v2.12.0"), pairFixture("v2.13.0"), pairFixture("v2.14.0")
	s := openPairFixture(t, root, first)
	a, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, a, archive, ManualUpdates)
	s = openPairFixture(t, root, second)
	b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, b, archive)
	require.NoError(t, s.ActivatePair(ctx))
	s = openPairFixture(t, root, third)
	c, archive := pairedBundleFixture(t, third, pairInfoFixture(third))
	preparePairFixture(t, s, c, archive)
	require.NoError(t, s.RollbackPair(ctx))
	require.NoError(t, s.Close())
	s = openPairFixture(t, root, third)
	require.NoError(t, s.RollbackPair(ctx))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, b, *status.Release, "recovery retry must not select the older A pair")
}

func TestPairActivationRetryAfterVisibleSelection(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runtime")
	first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
	s := openPairFixture(t, root, first)
	previous, oldArchive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, previous, oldArchive, ManualUpdates)
	s = openPairFixture(t, root, second)
	target, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, target, archive)
	original, calls := s.syncSelection, 0
	s.syncSelection = func() error {
		calls++
		if calls == 2 {
			return errors.New("injected directory fsync failure after selection")
		}
		return original()
	}
	var publication *SelectionError
	require.ErrorAs(t, s.ActivatePair(ctx), &publication)
	s.syncSelection = original
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, "activation-pending", status.Recovery.State)
	require.Equal(t, target, *status.Release)
	require.NoError(t, s.Close())
	s = openPairFixture(t, root, second)
	preparePairFixture(t, s, target, archive)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, "activation-pending", status.Recovery.State)
	require.NoError(t, s.ActivatePair(ctx))
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, "rollback-available", status.Recovery.State)
	require.Equal(t, previous, *status.Recovery.Previous)
}

func TestCompletedActivationRetryRechecksPayload(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runtime")
	first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
	s := openPairFixture(t, root, first)
	previous, oldArchive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, previous, oldArchive, ManualUpdates)
	s = openPairFixture(t, root, second)
	target, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, target, archive)
	require.NoError(t, s.ActivatePair(ctx))
	path := filepath.Join(root, "generations", target.Manifest.Archive.SHA256, releaseInfoName)
	require.NoError(t, os.Chmod(path, 0600))
	require.NoError(t, os.WriteFile(path, []byte("damaged identity"), 0600))
	require.ErrorIs(t, s.ActivatePair(ctx), ErrDamaged)
}

func TestCancelPreparationCannotRollBackAnExtraPair(t *testing.T) {
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "runtime")
	first, second, third := pairFixture("v2.12.0"), pairFixture("v2.13.0"), pairFixture("v2.14.0")
	s := openPairFixture(t, root, first)
	a, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, a, archive, ManualUpdates)
	s = openPairFixture(t, root, second)
	b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, b, archive)
	require.NoError(t, s.ActivatePair(ctx))
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	defer consumer.Close()
	require.NoError(t, s.CancelPreparation(ctx), "a failed preparation may not have created a pending record")
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, b, *status.Release)
	require.Equal(t, a, *status.Recovery.Previous)
	s = openPairFixture(t, root, third)
	c, archive := pairedBundleFixture(t, third, pairInfoFixture(third))
	preparePairFixture(t, s, c, archive)
	otherOwner := openPairFixture(t, root, second)
	require.ErrorIs(t, otherOwner.CancelPreparation(ctx), ErrIncompatible, "a failed competing owner cannot cancel another target")
	require.NoError(t, s.CancelPreparation(ctx), "canceling an unselected target does not require stopping old consumers")
	require.NoError(t, s.CancelPreparation(ctx))
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, b, *status.Release)
	require.Equal(t, b, *status.Recovery.Previous)
	require.Nil(t, status.Recovery.Prepared)
}

func TestUnreadableRecoveryEvidenceBlocksDestructiveOperations(t *testing.T) {
	for _, corrupt := range []string{"{}\n", "{\"format\":2}\n", "{\"format\":1,\"pending\":{}}\n", "broken"} {
		t.Run(corrupt, func(t *testing.T) {
			agent := pairFixture("v2.12.0")
			s := openPairFixture(t, filepath.Join(t.TempDir(), "runtime"), agent)
			release, archive := pairedBundleFixture(t, agent, pairInfoFixture(agent))
			installRelease(t, s, release, archive, ManualUpdates)
			require.NoError(t, os.WriteFile(filepath.Join(s.path, "recovery.json"), []byte(corrupt), 0600))
			for _, operation := range []func() error{s.Sweep, func() error { return s.Uninstall(context.Background()) },
				func() error { return s.RollbackPair(context.Background()) }} {
				err := operation()
				require.ErrorContains(t, err, "preserve recovery.json")
				require.NotErrorIs(t, err, ErrDamaged)
			}
			require.DirExists(t, filepath.Join(s.path, "generations", release.Manifest.Archive.SHA256))
			data, err := os.ReadFile(filepath.Join(s.path, "recovery.json"))
			require.NoError(t, err)
			require.Equal(t, corrupt, string(data))
		})
	}
}

func TestSameReleasePreparationPreservesPreviousRecoveryPoint(t *testing.T) {
	ctx := context.Background()
	first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
	root := filepath.Join(t.TempDir(), "runtime")
	s := openPairFixture(t, root, first)
	a, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, a, archive, ManualUpdates)
	s = openPairFixture(t, root, second)
	b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, b, archive)
	require.NoError(t, s.ActivatePair(ctx))
	before, err := os.ReadFile(filepath.Join(root, "recovery.json"))
	require.NoError(t, err)
	selectedBefore, err := os.ReadFile(filepath.Join(root, "selection.json"))
	require.NoError(t, err)
	candidate, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: PrepareRelease},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, b) })
	require.NoError(t, err)
	require.Nil(t, candidate, "identical preparation must not create a transaction")
	require.NoError(t, s.ActivatePair(ctx), "the owner can always finish its activation step")
	after, err := os.ReadFile(filepath.Join(root, "recovery.json"))
	require.NoError(t, err)
	require.Equal(t, before, after)
	selectedAfter, err := os.ReadFile(filepath.Join(root, "selection.json"))
	require.NoError(t, err)
	require.Equal(t, selectedBefore, selectedAfter)
	require.NoError(t, s.Sweep())
	require.DirExists(t, filepath.Join(root, "generations", a.Manifest.Archive.SHA256))
	require.NoError(t, s.RollbackPair(ctx))
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, a, *status.Release)
}

func TestSameReleasePreparationValidatesPayloadWithoutCreatingRecovery(t *testing.T) {
	for _, fault := range []string{"healthy", "missing executable", "damaged provenance"} {
		t.Run(fault, func(t *testing.T) {
			ctx := context.Background()
			agent := pairFixture("v2.13.0")
			s := openPairFixture(t, filepath.Join(t.TempDir(), "runtime"), agent)
			release, archive := pairedBundleFixture(t, agent, pairInfoFixture(agent))
			installRelease(t, s, release, archive, AutomaticUpdates)
			root := filepath.Join(s.path, "generations", release.Manifest.Archive.SHA256)
			switch fault {
			case "missing executable":
				path := filepath.Join(root, release.Manifest.Layout.Node)
				require.NoError(t, os.Rename(path, path+".damaged"))
			case "damaged provenance":
				path := filepath.Join(root, releaseInfoName)
				require.NoError(t, os.Chmod(path, 0600))
				require.NoError(t, os.WriteFile(path, []byte("damaged"), 0600))
			}
			candidate, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: PrepareRelease},
				func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, release) })
			require.Nil(t, candidate)
			if fault == "healthy" {
				require.NoError(t, err)
				require.NoError(t, s.ActivatePair(ctx), "initial selection needs no previous recovery point")
				other := openPairFixture(t, s.path, pairFixture("v2.14.0"))
				require.ErrorIs(t, other.ActivatePair(ctx), ErrIncompatible, "unchanged activation still requires the exact selected pair")
			} else {
				require.ErrorIs(t, err, ErrDamaged)
				require.ErrorIs(t, s.ActivatePair(ctx), ErrDamaged)
			}
			require.NoFileExists(t, filepath.Join(s.path, "recovery.json"))
		})
	}
}

func TestPairOperationsRejectCorruptedBindingWithoutChangingReferences(t *testing.T) {
	for _, operation := range []string{"unchanged", "preparation", "activation", "rollback"} {
		for _, fault := range []string{launcherName, fontConfigName, "node interpreter", "browser interpreter"} {
			t.Run(operation+"/"+fault, func(t *testing.T) {
				ctx := context.Background()
				first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
				path := filepath.Join(t.TempDir(), "runtime")
				s := openPairFixture(t, path, first)
				a, archive := pairedBundleFixture(t, first, pairInfoFixture(first))
				installRelease(t, s, a, archive, ManualUpdates)
				s = openPairFixture(t, path, second)
				b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
				var candidate *Candidate
				if operation == "preparation" {
					var err error
					candidate, err = s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: PrepareRelease},
						func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, b) })
					require.NoError(t, err)
					t.Cleanup(func() { require.NoError(t, candidate.Close()) })
					require.NoError(t, candidate.Import(ctx, archive))
				} else {
					preparePairFixture(t, s, b, archive)
					if operation != "activation" {
						require.NoError(t, s.ActivatePair(ctx))
					}
				}
				damagedManifest := b.Manifest
				if operation == "rollback" {
					damagedManifest = a.Manifest
				}
				root, err := s.root.OpenRoot(filepath.Join("generations", damagedManifest.Archive.SHA256))
				require.NoError(t, err)
				corruptBoundPayload(t, root, damagedManifest, fault)
				require.NoError(t, root.Close())
				selectionBefore, err := os.ReadFile(filepath.Join(path, "selection.json"))
				require.NoError(t, err)
				recoveryBefore, recoveryErr := os.ReadFile(filepath.Join(path, "recovery.json"))
				if operation == "preparation" {
					require.ErrorIs(t, recoveryErr, os.ErrNotExist)
				} else {
					require.NoError(t, recoveryErr)
				}
				switch operation {
				case "unchanged":
					reused, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: PrepareRelease},
						func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, b) })
					require.Nil(t, reused)
					require.ErrorIs(t, err, ErrDamaged)
					require.ErrorIs(t, s.ActivatePair(ctx), ErrDamaged)
				case "preparation":
					require.ErrorIs(t, candidate.PreparePair(), ErrDamaged)
				case "activation":
					require.ErrorIs(t, s.ActivatePair(ctx), ErrDamaged)
				case "rollback":
					require.ErrorIs(t, s.RollbackPair(ctx), ErrDamaged)
				}
				selectionAfter, err := os.ReadFile(filepath.Join(path, "selection.json"))
				require.NoError(t, err)
				require.Equal(t, selectionBefore, selectionAfter)
				recoveryAfter, err := os.ReadFile(filepath.Join(path, "recovery.json"))
				if recoveryErr != nil {
					require.ErrorIs(t, err, os.ErrNotExist)
				} else {
					require.NoError(t, err)
					require.Equal(t, recoveryBefore, recoveryAfter)
				}
			})
		}
	}
}

func TestRepairRestoredPairPreservesItsRecoveryReference(t *testing.T) {
	ctx := context.Background()
	first, second := pairFixture("v2.12.0"), pairFixture("v2.13.0")
	root := filepath.Join(t.TempDir(), "runtime")
	s := openPairFixture(t, root, first)
	a, firstArchive := pairedBundleFixture(t, first, pairInfoFixture(first))
	installRelease(t, s, a, firstArchive, ManualUpdates)
	s = openPairFixture(t, root, second)
	b, archive := pairedBundleFixture(t, second, pairInfoFixture(second))
	preparePairFixture(t, s, b, archive)
	require.NoError(t, s.ActivatePair(ctx))
	require.NoError(t, s.RollbackPair(ctx))
	s = openPairFixture(t, root, first)
	path := filepath.Join(root, "generations", a.Manifest.Archive.SHA256, releaseInfoName)
	require.NoError(t, os.Chmod(path, 0600))
	require.NoError(t, os.WriteFile(path, []byte("damaged identity"), 0600))
	_, err := s.AcquireSelected()
	require.ErrorIs(t, err, ErrDamaged)
	candidate, err := s.BeginPublishedInstall(ctx, ReleaseInstallOptions{Operation: RepairRelease, Updates: ManualUpdates},
		func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, a) })
	require.NoError(t, err, "the installation owner's rollback cannot repair damaged retained bytes")
	require.NoError(t, candidate.Import(ctx, firstArchive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	require.NoError(t, s.RollbackPair(ctx), "repair must preserve the same recovery reference")
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	require.NoError(t, consumer.Close())
}

func TestRetainedPairRepairRetryRequiresExactIdentity(t *testing.T) {
	ctx := context.Background()
	agent := pairFixture("v2.13.0")
	root := filepath.Join(t.TempDir(), "runtime")
	s := openPairFixture(t, root, agent)
	release, archive := pairedBundleFixture(t, agent, pairInfoFixture(agent))
	installRelease(t, s, release, archive, AutomaticUpdates)
	nextAgent := pairFixture("v2.14.0")
	s = openPairFixture(t, root, nextAgent)
	next, nextArchive := pairedBundleFixture(t, nextAgent, pairInfoFixture(nextAgent))
	preparePairFixture(t, s, next, nextArchive)
	require.NoError(t, s.CancelPreparation(ctx))
	s = openPairFixture(t, root, agent)
	recovery, err := os.ReadFile(filepath.Join(root, "recovery.json"))
	require.NoError(t, err)
	options := ReleaseInstallOptions{Operation: RepairRelease, Updates: ManualUpdates}
	load := func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, release) }
	candidate, err := s.BeginPublishedInstall(ctx, options, load)
	require.NoError(t, err)
	require.Error(t, candidate.Import(ctx, archive+"-missing"))
	require.NoError(t, candidate.Close())
	require.NoError(t, s.Close())
	generation := filepath.Join(root, "generations", release.Manifest.Archive.SHA256)
	require.NoDirExists(t, generation)
	selectionBefore, err := os.ReadFile(filepath.Join(root, "selection.json"))
	require.NoError(t, err)
	for _, mismatch := range []string{"manifest", "agent"} {
		t.Run(mismatch, func(t *testing.T) {
			target := release
			target.Agent = new(*release.Agent)
			if mismatch == "manifest" {
				target.Manifest.Archive.Bytes++
			} else {
				target.Agent.Source = strings.Repeat("b", 40)
			}
			other := openPairFixture(t, root, *target.Agent)
			candidate, err := other.BeginPublishedInstall(ctx, options,
				func(context.Context) (VerifiedRelease, error) { return verifyFixture(t, target) })
			require.ErrorContains(t, err, "differs from its retained pair")
			require.Nil(t, candidate)
			require.NoDirExists(t, generation)
			after, err := os.ReadFile(filepath.Join(root, "selection.json"))
			require.NoError(t, err)
			require.Equal(t, selectionBefore, after)
			after, err = os.ReadFile(filepath.Join(root, "recovery.json"))
			require.NoError(t, err)
			require.Equal(t, recovery, after)
		})
	}
	s = openPairFixture(t, root, agent)
	candidate, err = s.BeginPublishedInstall(ctx, options, load)
	require.NoError(t, err)
	require.NoError(t, candidate.Import(ctx, archive))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
	require.NoError(t, s.Sweep())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, release, *status.Release)
	require.False(t, status.AutomaticUpdates, "repair keeps its explicit consent policy")
	after, err := os.ReadFile(filepath.Join(root, "recovery.json"))
	require.NoError(t, err)
	require.Equal(t, recovery, after)
	require.NoError(t, s.RollbackPair(ctx))
	consumer, err := s.AcquireSelected()
	require.NoError(t, err)
	require.NoError(t, consumer.Close())
}
