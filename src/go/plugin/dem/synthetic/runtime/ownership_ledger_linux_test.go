// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/require"
)

func testOwnershipLedger(t *testing.T) *ownershipLedger {
	t.Helper()
	ledger, err := openOwnershipLedger(filepath.Join(t.TempDir(), "ledger"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ledger.Close()) })
	return ledger
}

func ledgerUse(t *testing.T, ledger *ownershipLedger, id string) *executionLease {
	t.Helper()
	var lease *executionLease
	require.NoError(t, ledger.withLock(func() error {
		var err error
		lease, err = ledger.newUseLocked(id)
		if err != nil {
			return err
		}
		return lease.publishLocked()
	}))
	return lease
}

func ledgerInventory(t *testing.T, ledger *ownershipLedger, reconcile bool) ownershipInventory {
	t.Helper()
	var inv ownershipInventory
	require.NoError(t, ledger.withLock(func() error {
		var err error
		inv, err = ledger.inventoryLocked(reconcile)
		return err
	}))
	return inv
}

func TestOwnershipLedgerIndependentOfPayload(t *testing.T) {
	ledger := testOwnershipLedger(t)
	for _, name := range []string{"generations", "staging", "selection.json", "installation.lock"} {
		_, err := ledger.root.Lstat(name)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
	id := strings.Repeat("a", 64)
	var process, candidate *claim
	require.NoError(t, ledger.withLock(func() error {
		var err error
		process, err = ledger.claimLocked(id, "process")
		if err != nil {
			return err
		}
		candidate, err = ledger.claimLocked(id, "candidate")
		return err
	}))
	lease := ledgerUse(t, ledger, id)
	inv := ledgerInventory(t, ledger, false)
	require.Equal(t, 1, inv.processes[id])
	require.Equal(t, 1, inv.candidates[id])
	require.Equal(t, 1, inv.uses[id])
	require.Empty(t, inv.uncertain)
	require.NoError(t, process.Close())
	require.NoError(t, candidate.Close())
	require.NoError(t, lease.Finish(false))

	other, err := openOwnershipLedger(ledger.path)
	require.NoError(t, err)
	defer other.Close()
	inv = ledgerInventory(t, other, true)
	require.Empty(t, inv.processes)
	require.Empty(t, inv.candidates)
	require.Equal(t, 1, inv.uses[id], "local lock release does not prove descendant completion")
	require.Equal(t, 1, inv.uncertain[id])
}

func TestOwnershipLedgerReceiptAndBootRecovery(t *testing.T) {
	for _, proof := range []string{"receipt", "reboot"} {
		t.Run(proof, func(t *testing.T) {
			ledger := testOwnershipLedger(t)
			ledger.boot = bootIdentity{ID: "11111111-1111-1111-1111-111111111111", Verified: true}
			id := strings.Repeat("b", 64)
			lease := ledgerUse(t, ledger, id)
			if proof == "receipt" {
				helper := os.Getenv("DEM_RUNTIME_TEST_HELPER")
				if helper == "" {
					require.NoError(t, lease.Finish(true))
					t.Skip("real-helper test requires DEM_RUNTIME_TEST_HELPER")
				}
				t.Cleanup(ndexec.SetRunnerPathsForTests(helper, ""))
				process, err := ndexec.StartUnprivilegedProcessTree(context.Background(), ndexec.TreeOptions{Completion: lease.Completion}, "/bin/true")
				require.NoError(t, err)
				require.True(t, process.Wait().Drained)
			}
			require.NoError(t, lease.Finish(false))
			if proof == "reboot" {
				ledger.boot = bootIdentity{ID: "22222222-2222-2222-2222-222222222222", Verified: false}
				require.Equal(t, 1, ledgerInventory(t, ledger, true).uncertain[id])
				ledger.boot.Verified = true
			}
			require.Empty(t, ledgerInventory(t, ledger, true).uses)
		})
	}
}

func TestOwnershipLedgerMalformedProofFailsClosed(t *testing.T) {
	for _, record := range []string{"owner.json", "missing-completion", "malformed-completion"} {
		t.Run(record, func(t *testing.T) {
			ledger := testOwnershipLedger(t)
			id := strings.Repeat("c", 64)
			lease := ledgerUse(t, ledger, id)
			require.NoError(t, lease.Finish(false))
			if record == "owner.json" {
				require.NoError(t, os.WriteFile(filepath.Join(ledger.path, lease.name, record), []byte("{broken"), 0600))
				require.Error(t, ledger.withLock(func() error { _, err := ledger.inventoryLocked(true); return err }))
			} else {
				if record == "malformed-completion" {
					require.NoError(t, os.WriteFile(filepath.Join(ledger.path, lease.name, "complete"), []byte("NDTREE1"), 0600))
				}
				// Missing or damaged completion is uncertain, never empty.
				inv := ledgerInventory(t, ledger, true)
				require.Equal(t, 1, inv.uses[id])
				require.Equal(t, 1, inv.uncertain[id])
				if record == "malformed-completion" {
					require.Error(t, inv.cleanupError)
				}
			}
			require.DirExists(t, filepath.Join(ledger.path, lease.name))
		})
	}
}

func TestOwnershipLedgerNeverReinitializesReadyState(t *testing.T) {
	for _, name := range []string{layoutName, "coordination.lock", "claims", "uses", "initializing", "trash"} {
		t.Run(name, func(t *testing.T) {
			ledger := testOwnershipLedger(t)
			path := filepath.Join(ledger.path, name)
			require.NoError(t, os.Rename(path, path+".saved"))
			other, err := openOwnershipLedger(ledger.path)
			require.Error(t, err)
			require.Nil(t, other)
			_, err = os.Lstat(path)
			require.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestOpenOwnersDoNotRecreateLostCoordinationLocks(t *testing.T) {
	t.Run("ledger", func(t *testing.T) {
		ledger := testOwnershipLedger(t)
		path := filepath.Join(ledger.path, "coordination.lock")
		require.NoError(t, os.Rename(path, path+".lost"))
		err := ledger.withLock(func() error { return nil })
		require.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Lstat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
	t.Run("installation", func(t *testing.T) {
		s, _ := testStore(t)
		path := filepath.Join(s.path, "installation.lock")
		require.NoError(t, os.Rename(path, path+".lost"))
		m, _ := bundleFixture(t, "lost-lock")
		candidate, err := s.BeginInstall(context.Background(), m)
		if candidate != nil {
			require.NoError(t, candidate.Close())
		}
		require.ErrorIs(t, err, os.ErrNotExist)
		_, err = os.Lstat(path)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}

func TestOwnershipLedgerBootstrapAndLayoutIsolation(t *testing.T) {
	path := t.TempDir()
	require.NoError(t, os.Chmod(path, 0700))
	root, err := os.OpenRoot(path)
	require.NoError(t, err)
	require.NoError(t, createInitialLayout(root, storeLayout{Format: int(ownershipRootLayout), Phase: "initializing"}))
	require.NoError(t, root.Close())
	ledger, err := openOwnershipLedger(path)
	require.NoError(t, err)
	defer ledger.Close()
	record, err := ownershipRootLayout.read(ledger.root)
	require.NoError(t, err)
	require.Equal(t, "ready", record.Phase)
	managed, err := Open(DefaultConfig(path))
	require.Error(t, err)
	require.Nil(t, managed)
	s, _ := testStore(t)
	other, err := openOwnershipLedger(s.path)
	require.Error(t, err)
	require.Nil(t, other)
}

func TestOwnershipLedgerAbortedPublicationRetainsEmptyProof(t *testing.T) {
	ledger := testOwnershipLedger(t)
	var lease *executionLease
	require.NoError(t, ledger.withLock(func() error {
		var err error
		lease, err = ledger.newUseLocked(strings.Repeat("d", 64))
		return err
	}))
	// Publication completed, but the payload owner has not returned a path.
	// Its abort must retain verified-empty proof if record retirement fails.
	require.NoError(t, ledger.withLock(lease.publishLocked))
	trash := filepath.Join(ledger.path, "trash")
	require.NoError(t, os.Rename(trash, trash+".saved"))
	require.NoError(t, os.WriteFile(trash, nil, 0600))
	require.Error(t, ledger.withLock(lease.abortLocked))
	require.True(t, ledger.completedUses[lease.name])
	require.NoError(t, os.Rename(trash, trash+".blocked"))
	require.NoError(t, os.Rename(trash+".saved", trash))
	require.Empty(t, ledgerInventory(t, ledger, true).uses)
	require.Empty(t, ledger.completedUses)
}

func TestOwnershipLedgerTerminalReleaseAndHealthRetry(t *testing.T) {
	ledger := testOwnershipLedger(t)
	lease := ledgerUse(t, ledger, strings.Repeat("e", 64))
	closer := &retryOwnerClose{Closer: lease.lock, fail: true}
	lease.lock = closer
	version := ledger.cleanupErrorVersion
	require.Error(t, lease.Finish(true))
	ledger.finishSweep(version, nil)
	require.Error(t, ledger.cleanupError, "an older successful sweep must not erase terminal failure")
	require.Error(t, ledger.withLock(ledger.retryOwnerReleasesLocked))
	closer.fail = false
	require.NoError(t, ledger.withLock(ledger.retryOwnerReleasesLocked))
	require.Empty(t, ledgerInventory(t, ledger, true).uses)
	var names []string
	require.NoError(t, ledger.withLock(func() error { var err error; names, err = ledger.trashSnapshotLocked(); return err }))
	require.NoError(t, ledger.deleteTrash(names))
	ledger.finishSweep(ledger.cleanupErrorVersion, nil)
	require.NoError(t, ledger.cleanupError)
	require.Empty(t, ledger.releasingOwners)
	require.Error(t, ledger.withLock(func() error {
		return ledger.removeOwned("generations/" + strings.Repeat("f", 64))
	}), "the ledger cannot authorize managed payload retirement")
}
