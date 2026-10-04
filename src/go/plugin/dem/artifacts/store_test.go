// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var run1 = fixtureRunID()
var run2 = fixtureRunID()

func fixtureRunID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

var png = []byte("\x89PNG\r\n\x1a\nsynthetic-test-content")

func store(t *testing.T) (*artifacts.Store, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "artifacts")
	s, err := artifacts.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, s.Close())
		// These fixtures launch no child. Remove only this test's exact aliases.
		for _, id := range []string{run1, run2} {
			alias := filepath.Join("/tmp", "nd-dem-"+id)
			if target, err := os.Readlink(alias); err == nil && target == filepath.Join(root, "work", id) {
				require.NoError(t, os.Remove(alias))
			}
		}
	})
	return s, root
}
func captured(t *testing.T, s *artifacts.Store, id string, content []byte) (string, synthetic.Capture) {
	t.Helper()
	work, err := s.Begin(id)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "report.html"), content, 0600))
	return work, synthetic.Capture{ID: "lighthouse-report", Kind: "report", MIME: "text/html", Path: "output/report.html"}
}
func TestPublishFetchAndRestart(t *testing.T) {
	ctx := context.Background()
	s, root := store(t)
	payload := []byte("<!doctype html><p>synthetic</p>")
	work, c := captured(t, s, run1, payload)
	require.NoError(t, os.WriteFile(filepath.Join(work, "secret-script"), []byte("task secret"), 0600))
	out, err := s.Finalize(ctx, run1, []synthetic.Capture{c})
	require.NoError(t, err)
	require.Len(t, out, 1)
	hash := sha256.Sum256(payload)
	assert.Equal(t, int64(len(payload)), out[0].Bytes)
	assert.Equal(t, hex.EncodeToString(hash[:]), out[0].SHA256)
	_, err = os.Stat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	meta, got, err := s.Fetch(ctx, run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, out[0], meta)
	assert.Equal(t, payload, got)
	assert.Positive(t, s.Stats().RetainedBytes)
	assert.Zero(t, s.Stats().ProtectedBytes)
	index, err := s.Manifest(ctx, run1)
	require.NoError(t, err)
	assert.Equal(t, out, index)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, got, err = reopened.Fetch(ctx, run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	_, err = reopened.Begin(run1)
	assert.ErrorIs(t, err, artifacts.ErrExists)
}
func TestStrictCapturePathsAndKinds(t *testing.T) {
	for _, which := range []string{"escape", "absolute", "clean", "link-file", "link-parent", "directory", "wrong-mime", "wrong-kind", "bad-png", "duplicate"} {
		t.Run(which, func(t *testing.T) {
			s, _ := store(t)
			work, c := captured(t, s, run1, []byte("report"))
			outside := filepath.Join(t.TempDir(), "outside")
			require.NoError(t, os.WriteFile(outside, []byte("outside remains"), 0600))
			switch which {
			case "escape":
				c.Path = "output/../../outside"
			case "absolute":
				c.Path = outside
			case "clean":
				c.Path = "output/./report.html"
			case "link-file":
				require.NoError(t, os.Symlink(outside, filepath.Join(work, "output", "link")))
				c.Path = "output/link"
			case "link-parent":
				require.NoError(t, os.Symlink(filepath.Dir(outside), filepath.Join(work, "output", "link")))
				c.Path = "output/link/outside"
			case "directory":
				c.Path = "output"
			case "wrong-mime":
				c.MIME = "text/plain"
			case "wrong-kind":
				c.Kind = "trace"
			case "bad-png":
				c.Kind = "screenshot"
				c.MIME = "image/png"
			}
			captures := []synthetic.Capture{c}
			if which == "duplicate" {
				captures = append(captures, c)
			}
			_, err := s.Finalize(context.Background(), run1, captures)
			assert.ErrorIs(t, err, artifacts.ErrInvalidCapture)
			_, err = s.Enforce(context.Background(), 1, 1)
			require.NoError(t, err)
			_, err = os.Stat(work)
			assert.NoError(t, err, "failed publication remains protected for this owner")
			got, err := os.ReadFile(outside)
			require.NoError(t, err)
			assert.Equal(t, "outside remains", string(got))
			assert.Positive(t, s.Stats().ProtectedBytes)
		})
	}
}
func TestPNGCapture(t *testing.T) {
	s, _ := store(t)
	work, err := s.Begin(run1)
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "capture.png"), png, 0600))
	c := synthetic.Capture{ID: "screenshot-1", Kind: "screenshot", MIME: "image/png", Path: "output/capture.png"}
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	_, got, err := s.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, png, got)
}
func TestFetchBoundAndCorruption(t *testing.T) {
	s, root := store(t)
	_, c := captured(t, s, run1, []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes+1))))
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	_, _, err = s.Fetch(context.Background(), run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrTooLarge)
	_, c = captured(t, s, run2, []byte("good"))
	_, err = s.Finalize(context.Background(), run2, []synthetic.Capture{c})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "runs", run2, c.ID), []byte("bad!"), 0600))
	_, _, err = s.Fetch(context.Background(), run2, c.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest")
}
func TestRetentionAndProtectedWork(t *testing.T) {
	s, root := store(t)
	_, c := captured(t, s, run1, []byte("retained report"))
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	work, err := s.Begin(run2)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(work, "profile"), []byte("unverified browser state"), 0600))
	stats, err := s.Enforce(context.Background(), 7, 2<<30)
	require.NoError(t, err)
	assert.Positive(t, stats.ProtectedBytes)
	_, _, err = s.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err, "fresh capture remains inside policy")
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, _, err = s.Fetch(context.Background(), run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Stat(work)
	assert.NoError(t, err, "age, budget and restart never prove drainage")
	_, _, err = reopened.Fetch(context.Background(), run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
}
func TestDrainedFailedPublicationIsRecoveredAfterRestart(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte("invalid candidate"))
	c.Path = "outside"
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.Error(t, err)
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Stat(work)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Stat(work)
	assert.ErrorIs(t, err, os.ErrNotExist, "durable marker proves cleanup permission")
	assert.Zero(t, reopened.Stats().ProtectedBytes)
}
func TestFetchExpiryRace(t *testing.T) {
	s, _ := store(t)
	payload := []byte(strings.Repeat("x", 50000))
	_, c := captured(t, s, run1, payload)
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	var wg sync.WaitGroup
	for range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, got, err := s.Fetch(context.Background(), run1, c.ID)
			if err == nil {
				assert.Equal(t, payload, got)
			} else {
				assert.ErrorIs(t, err, artifacts.ErrNotFound)
			}
		}()
	}
	wg.Add(1)
	go func() { defer wg.Done(); _, err := s.Enforce(context.Background(), 7, 1); assert.NoError(t, err) }()
	wg.Wait()
}
func TestOwnershipIDsAndClose(t *testing.T) {
	s, root := store(t)
	other, err := artifacts.Open(root)
	assert.Nil(t, other)
	assert.ErrorIs(t, err, artifacts.ErrLocked)
	for _, id := range []string{"../escape", "ABCDEF", "short", "0000000000000000000000000000000g"} {
		_, err = s.Begin(id)
		assert.ErrorIs(t, err, artifacts.ErrInvalidID)
	}
	_, err = s.Begin(run1)
	require.NoError(t, err)
	_, err = s.Begin(run1)
	assert.ErrorIs(t, err, artifacts.ErrExists)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Finalize(ctx, run1, nil)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.Finalize(context.Background(), run1, nil)
	require.NoError(t, err)
	_, _, err = s.Fetch(context.Background(), run1, "absent")
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	_, err = s.Begin(run2)
	assert.ErrorIs(t, err, artifacts.ErrClosed)
}
func TestTemporaryClosePreservesUnverifiedWork(t *testing.T) {
	s, err := artifacts.Open("")
	require.NoError(t, err)
	work, err := s.Begin(run1)
	require.NoError(t, err)
	actual, err := os.Readlink(work)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	_, err = os.Stat(work)
	require.NoError(t, err)
	// This fixture owns the simulated work; no child process was started.
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.RemoveAll(filepath.Dir(filepath.Dir(actual))))
	s, err = artifacts.Open("")
	require.NoError(t, err)
	work, err = s.Begin(run1)
	require.NoError(t, err)
	actual, err = os.Readlink(work)
	require.NoError(t, err)
	_, err = s.Finalize(context.Background(), run1, nil)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	_, err = os.Stat(filepath.Dir(filepath.Dir(actual)))
	assert.True(t, errors.Is(err, os.ErrNotExist))
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func TestInvalidDrainMarkerCannotAuthorizeCleanup(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte("retained uncertainty"))
	c.Path = "invalid"
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.Error(t, err)
	require.NoError(t, s.Close())
	// Corrupt a real published marker to model interrupted persistence.
	require.NoError(t, os.WriteFile(filepath.Join(root, "drained", run1), nil, 0600))
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	stats, err := reopened.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	assert.Positive(t, stats.CleanupErrors)
	assert.Positive(t, stats.ProtectedBytes)
	_, err = os.Stat(work)
	assert.NoError(t, err)
}
func TestFetchExactSizeBoundary(t *testing.T) {
	s, _ := store(t)
	payload := []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)))
	_, c := captured(t, s, run1, payload)
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	_, got, err := s.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
}

func TestShortAliasKeepsLongPrivateWork(t *testing.T) {
	root := filepath.Join(t.TempDir(), strings.Repeat("long-root-", 15))
	s, err := artifacts.Open(root)
	require.NoError(t, err)
	defer s.Close()
	work, c := captured(t, s, run1, []byte("long-path report"))
	assert.Less(t, len(work), 60)
	target, err := os.Readlink(work)
	require.NoError(t, err)
	assert.Greater(t, len(target), 120)
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, raw, err := s.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "long-path report", string(raw))
}
func TestExistingAliasIsNeverAdoptedOrRemoved(t *testing.T) {
	s, _ := store(t)
	alias := filepath.Join("/tmp", "nd-dem-"+run1)
	unrelated := t.TempDir()
	require.NoError(t, os.Symlink(unrelated, alias))
	defer os.Remove(alias)
	_, err := s.Begin(run1)
	require.Error(t, err)
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	target, err := os.Readlink(alias)
	require.NoError(t, err)
	assert.Equal(t, unrelated, target)
}
func TestMismatchedAliasBlocksDrainedCleanup(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte("candidate"))
	actual, err := os.Readlink(work)
	require.NoError(t, err)
	require.NoError(t, os.Remove(work))
	unrelated := t.TempDir()
	require.NoError(t, os.Symlink(unrelated, work))
	defer os.Remove(work)
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.Error(t, err)
	_, err = s.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	target, err := os.Readlink(work)
	require.NoError(t, err)
	assert.Equal(t, unrelated, target)
	_, err = os.Stat(actual)
	require.NoError(t, err)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 1)
	require.Error(t, err)
	target, err = os.Readlink(work)
	require.NoError(t, err)
	assert.Equal(t, unrelated, target)
}
