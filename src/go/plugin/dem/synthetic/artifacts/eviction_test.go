// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func transitionCapture(t *testing.T, s *Store) string {
	t.Helper()
	var raw [16]byte
	_, err := rand.Read(raw[:])
	require.NoError(t, err)
	id := hex.EncodeToString(raw[:])
	work, err := s.Begin(id, "lighthouse:test")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "report"), []byte("<html>diagnosis</html>"), 0600))
	_, err = s.Finalize(
		context.Background(),
		id,
		[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report"}},
	)
	require.NoError(t, err)
	return id
}

func TestEvictionFailureRecoveryKeepsDurableAuthorization(t *testing.T) {
	for _, point := range []string{"before-rename", "after-rename", "runs-sync", "deleting-sync", "partial-remove", "final-sync"} {
		t.Run(point, func(t *testing.T) {
			root := t.TempDir()
			s, err := Open(root)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			id := transitionCapture(t, s)
			real := s.eviction
			fault := errors.New("injected filesystem failure")
			moved, removed := false, false
			s.eviction.rename = func(from, to string) error {
				if point == "before-rename" {
					return fault
				}
				if err := real.rename(from, to); err != nil {
					return err
				}
				moved = true
				if point == "after-rename" {
					return fault
				}
				return nil
			}
			s.eviction.sync = func(dir string) error {
				if moved &&
					((point == "runs-sync" && dir == "runs") || (point == "deleting-sync" && dir == "deleting" && !removed) || (point == "final-sync" && dir == "deleting" && removed)) {
					return fault
				}
				return real.sync(dir)
			}
			s.eviction.removeAll = func(name string) error {
				if point == "partial-remove" {
					require.NoError(t, s.root.Remove(name+"/manifest.json"))
					return fault
				}
				err := real.removeAll(name)
				removed = err == nil
				return err
			}
			status, err := s.Enforce(context.Background(), 7, 1)
			require.ErrorIs(t, err, fault)
			assert.Contains(t, status.Cleanup.Error, fault.Error())
			if point == "before-rename" {
				_, _, err = s.Fetch(context.Background(), "lighthouse:test", id, "report")
				require.NoError(t, err)
			} else {
				_, _, err = s.Fetch(context.Background(), "lighthouse:test", id, "report")
				require.ErrorIs(t, err, ErrNotFound)
			}
			if point == "partial-remove" {
				_, err = os.Stat(filepath.Join(root, "deleting", id, "manifest.json"))
				require.ErrorIs(t, err, os.ErrNotExist)
				_, err = os.Stat(filepath.Join(root, "deleting", id, "report"))
				require.NoError(t, err)
			}
			require.NoError(t, s.Close())
			reopened, err := Open(root)
			require.NoError(t, err)
			defer reopened.Close()
			status, err = reopened.Enforce(context.Background(), 7, 1)
			require.NoError(t, err)
			require.NotNil(t, status.Inventory.Deleting.Bytes)
			assert.Zero(t, *status.Inventory.Deleting.Bytes)
			assert.Empty(t, status.Cleanup.Error)
			assert.Positive(t, status.Cleanup.SuccessfulUS)
			for _, dir := range []string{"runs", "deleting"} {
				entries, err := os.ReadDir(filepath.Join(root, dir))
				require.NoError(t, err)
				assert.Empty(t, entries)
			}
		})
	}
}

func TestRetrySyncsParentsBeforeRemovalAndAfterEmptyFailure(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	id := transitionCapture(t, s)
	real := s.eviction
	fault := errors.New("sync unavailable")
	// Rename really occurs; interruption leaves its parent durability incomplete.
	s.eviction.rename = func(from, to string) error {
		if err := real.rename(from, to); err != nil {
			return err
		}
		return fault
	}
	_, err = s.Enforce(context.Background(), 7, 1)
	require.ErrorIs(t, err, fault)
	s.eviction = real
	var events []string
	s.eviction.sync = func(dir string) error { events = append(events, "sync:"+dir); return real.sync(dir) }
	s.eviction.removeAll = func(name string) error {
		require.GreaterOrEqual(t, len(events), 2)
		assert.Equal(t, []string{"sync:runs", "sync:deleting"}, events[len(events)-2:])
		events = append(events, "remove:"+name)
		return real.removeAll(name)
	}
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	assert.Contains(t, events, "remove:deleting/"+id)
	assert.Equal(t, "sync:deleting", events[len(events)-1])
	// A later sync failure must be retried even without an entry left to visit.
	calls := 0
	s.eviction.sync = func(dir string) error { calls++; return fault }
	_, err = s.Enforce(context.Background(), 7, 1)
	require.ErrorIs(t, err, fault)
	assert.Positive(t, calls)
	s.eviction = real
	status, err := s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	assert.Empty(t, status.Cleanup.Error)
}

func TestDeletionCollisionAndUnknownEntriesArePreserved(t *testing.T) {
	s, err := Open("")
	require.NoError(t, err)
	root := s.path
	// Test-owned store launches no child; remove its preserved fixture after Close.
	t.Cleanup(func() { require.NoError(t, s.Close()); require.NoError(t, os.RemoveAll(root)) })
	id := transitionCapture(t, s)
	require.NoError(t, s.root.Mkdir("deleting/"+id, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "deleting", id, "unrelated"), []byte("preserve"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "deleting", "unknown"), []byte("preserve"), 0600))
	_, err = s.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", id, "report")
	require.NoError(t, err)
	for _, name := range []string{id + "/unrelated", "unknown"} {
		body, err := os.ReadFile(filepath.Join(root, "deleting", name))
		require.NoError(t, err)
		assert.Equal(t, "preserve", string(body))
	}
	_, err = s.Begin(id, "lighthouse:test")
	require.ErrorIs(t, err, ErrExists)
	require.NoError(t, s.Close())
	_, err = os.Stat(root)
	require.NoError(t, err, "Close must honor preservation promised by maintenance")
}

func TestInventoryFailureDoesNotStopOpenOrRunLocalPublication(t *testing.T) {
	root := t.TempDir()
	s, err := Open(root)
	require.NoError(t, err)
	id := transitionCapture(t, s)
	require.NoError(t, s.Close())
	// A damaged set has no usable ownership. It stays unclassified, not expired.
	require.NoError(t, os.Remove(filepath.Join(root, "runs", id, "manifest.json")))
	s, err = Open(root)
	require.NoError(t, err)
	defer s.Close()
	_, err = s.Manifest(context.Background(), "lighthouse:test", id)
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrNotFound)
	fresh := transitionCapture(t, s)
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", fresh, "report")
	require.NoError(t, err)
	status, err := s.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	require.NotNil(t, status.Inventory.Unclassified.Bytes)
	assert.Positive(t, *status.Inventory.Unclassified.Bytes)
	_, err = os.Stat(filepath.Join(root, "runs", id, "report"))
	require.NoError(t, err)
	_, err = s.Manifest(context.Background(), "lighthouse:test", fresh)
	assert.ErrorIs(t, err, ErrNotFound, "known sets still follow policy")
}

func TestCancellationFinishesStartedEviction(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	id := transitionCapture(t, s)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	real := s.eviction.rename
	s.eviction.rename = func(from, to string) error {
		err := real(from, to)
		cancel()
		return err
	}
	_, err = s.Enforce(ctx, 7, 1)
	require.ErrorIs(t, err, context.Canceled)
	for _, dir := range []string{"runs", "deleting"} {
		_, err = s.root.Lstat(dir + "/" + id)
		assert.ErrorIs(t, err, os.ErrNotExist)
	}
	status, err := s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	assert.Empty(t, status.Cleanup.Error)
}

func TestPendingDeletionAloneReservesRunID(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	id := transitionCapture(t, s)
	real := s.eviction
	s.eviction.removeAll = func(string) error { return errors.New("removal unavailable") }
	_, err = s.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	_, err = s.root.Lstat("runs/" + id)
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = s.Begin(id, "lighthouse:test")
	assert.ErrorIs(t, err, ErrExists)
	s.eviction = real
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
}

func TestEmptyCaptureCleanupFailureDoesNotChangeCompletion(t *testing.T) {
	s, err := Open(t.TempDir())
	require.NoError(t, err)
	defer s.Close()
	var raw [16]byte
	_, err = rand.Read(raw[:])
	require.NoError(t, err)
	id := hex.EncodeToString(raw[:])
	work, err := s.Begin(id, "journey:test")
	require.NoError(t, err)
	// Replace only this test-owned alias; no child was launched.
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(t.TempDir(), work))
	defer os.Remove(work)
	captures, err := s.Finalize(context.Background(), id, nil)
	require.NoError(t, err)
	assert.Empty(t, captures)
	page, err := s.Catalogue(context.Background(), Filter{
		Limit: 1,
	})
	require.NoError(t, err)
	assert.Empty(t, page.Sets)
	assert.Contains(t, page.Status.Cleanup.Error, "ownership does not match")
	_, err = s.root.Lstat("drained/" + id)
	require.NoError(t, err)
	require.NoError(t, os.Remove(work))
	status, err := s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	assert.Empty(t, status.Cleanup.Error)
	require.NotNil(t, status.Inventory.Workspace.Bytes)
	assert.Zero(t, *status.Inventory.Workspace.Bytes)
}
