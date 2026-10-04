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
			s, root := store(t)
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
			assert.ErrorIs(t, err, os.ErrNotExist, "drained unpublished work is removed")
			for _, dir := range []string{"work", "staging", "drained", "runs"} {
				_, err = os.Lstat(filepath.Join(root, dir, run1))
				assert.ErrorIs(t, err, os.ErrNotExist, dir)
			}
			got, err := os.ReadFile(outside)
			require.NoError(t, err)
			assert.Equal(t, "outside remains", string(got))
			assert.Zero(t, s.Stats().ProtectedBytes)
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
	work, c := captured(t, s, run1, []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes+1))))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "small.html"), []byte("small"), 0600))
	small := synthetic.Capture{ID: "small-report", Kind: "report", MIME: "text/html", Path: "output/small.html"}
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{small, c})
	require.ErrorIs(t, err, artifacts.ErrTooLarge)
	_, _, err = s.Fetch(context.Background(), run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	assert.Zero(t, s.Stats().ProtectedBytes)
	for _, dir := range []string{"work", "staging", "drained", "runs"} {
		_, err = os.Lstat(filepath.Join(root, dir, run1))
		assert.ErrorIs(t, err, os.ErrNotExist, dir)
	}
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
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
func TestDrainedFailedCaptureCleanupRetries(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte("invalid candidate"))
	actual, err := os.Readlink(work)
	require.NoError(t, err)
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(t.TempDir(), work))
	defer os.Remove(work)
	c.Path = "outside"
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.ErrorIs(t, err, artifacts.ErrInvalidCapture)
	assert.Contains(t, err.Error(), "ownership does not match")
	assert.Positive(t, s.Stats().CleanupErrors)
	_, err = os.Stat(filepath.Join(root, "drained", run1))
	require.NoError(t, err)
	// Restore this fixture's alias and retry in the same owner, without restart.
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(actual, work))
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, s.Stats().ProtectedBytes)
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
	actual, err := os.Readlink(work)
	require.NoError(t, err)
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(t.TempDir(), work))
	defer os.Remove(work)
	c.Path = "invalid"
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.ErrorIs(t, err, artifacts.ErrInvalidCapture)
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(actual, work))
	require.NoError(t, s.Close())
	// Corrupt a real marker whose cleanup failed to model interrupted persistence.
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
	s, root := store(t)
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
	_, err = os.Stat(filepath.Join(root, "work", run1))
	assert.ErrorIs(t, err, os.ErrNotExist, "failed setup must remove its new empty work")
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
	published, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err, "owned-work cleanup does not undo durable publication")
	require.Len(t, published, 1)
	_, raw, err := s.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "candidate", string(raw))
	assert.Positive(t, s.Stats().CleanupErrors)
	_, err = s.Enforce(context.Background(), 7, 2<<30)
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
	_, err = reopened.Enforce(context.Background(), 7, 2<<30)
	require.Error(t, err)
	target, err = os.Readlink(work)
	require.NoError(t, err)
	assert.Equal(t, unrelated, target)
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(actual, work))
	_, err = reopened.Enforce(context.Background(), 7, 2<<30)
	require.NoError(t, err)
	assert.Zero(t, reopened.Stats().ProtectedBytes)
	_, raw, err = reopened.Fetch(context.Background(), run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "candidate", string(raw))
}

// Trigger real filesystem errors without changing shared directories. Skip
// systems/users that do not enforce directory read permissions.
func denyDirectoryReads(t *testing.T, dir string) {
	t.Helper()
	require.NoError(t, os.Chmod(dir, 0300))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0700)) })
	f, err := os.Open(dir)
	if err == nil {
		require.NoError(t, f.Close())
		t.Skip("directory read permissions are not enforced")
	}
	require.ErrorIs(t, err, os.ErrPermission)
}

func TestFailedPublicationAttemptStaysProtectedUntilRestart(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte("published candidate"))
	runs := filepath.Join(root, "runs")
	denyDirectoryReads(t, runs)
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.ErrorIs(t, err, os.ErrPermission)
	require.NoError(t, os.Chmod(runs, 0700))
	_, err = os.Stat(filepath.Join(root, "staging", run1, c.ID))
	require.NoError(t, err, "completed staging remains protected after a failed publication attempt")
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Stat(work)
	require.NoError(t, err, "uncertain publication preserves original work")
	_, err = s.Manifest(context.Background(), run1)
	require.ErrorContains(t, err, "uncertain")
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 2<<30)
	require.NoError(t, err)
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, _, err = reopened.Fetch(context.Background(), run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	assert.Zero(t, reopened.Stats().ProtectedBytes)
}

// Grow the fixture at the copy's first cancellation check, after its initial Stat.
type growBeforeReadContext struct {
	context.Context
	checks int
	grow   func()
}

func (c *growBeforeReadContext) Err() error {
	c.checks++
	if c.checks == 3 {
		c.grow()
	}
	return c.Context.Err()
}

func TestCaptureGrowingPastFetchLimitIsNotPublished(t *testing.T) {
	s, root := store(t)
	work, c := captured(t, s, run1, []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes))))
	ctx := &growBeforeReadContext{Context: context.Background(), grow: func() {
		file, err := os.OpenFile(filepath.Join(work, c.Path), os.O_APPEND|os.O_WRONLY, 0600)
		require.NoError(t, err)
		_, err = file.Write([]byte("extra"))
		require.NoError(t, err)
		require.NoError(t, file.Close())
	}}
	_, err := s.Finalize(ctx, run1, []synthetic.Capture{c})
	require.ErrorIs(t, err, artifacts.ErrTooLarge)
	for _, dir := range []string{"work", "staging", "drained", "runs"} {
		_, err = os.Lstat(filepath.Join(root, dir, run1))
		assert.ErrorIs(t, err, os.ErrNotExist, dir)
	}
	assert.Zero(t, s.Stats().ProtectedBytes)
}
