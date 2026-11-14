// SPDX-License-Identifier: GPL-3.0-or-later

package artifacts_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
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
	work, err := s.Begin(id, "lighthouse:test", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "report.html"), content, 0600))
	return work, synthetic.Capture{
		ID:   "lighthouse-report",
		Kind: "report",
		MIME: "text/html",
		Path: "output/report.html",
	}
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
	meta, got, err := s.Fetch(ctx, "lighthouse:test", run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, out[0], meta)
	assert.Equal(t, payload, got)
	assert.Positive(t, *observedStatus(t, s).Inventory.Retained.Bytes)
	assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
	index, err := s.Manifest(ctx, "lighthouse:test", run1)
	require.NoError(t, err)
	assert.Equal(t, out, index.Artifacts)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, got, err = reopened.Fetch(ctx, "lighthouse:test", run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, payload, got)
	_, err = reopened.Begin(run1, "lighthouse:test", "")
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
			assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
		})
	}
}

func TestPNGCapture(t *testing.T) {
	s, _ := store(t)
	work, err := s.Begin(run1, "lighthouse:test", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "output", "capture.png"), png, 0600))
	c := synthetic.Capture{
		ID:   "screenshot-1",
		Kind: "screenshot",
		MIME: "image/png",
		Path: "output/capture.png",
	}
	_, err = s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	_, got, err := s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, png, got)
}

func TestOversizedCapturesPreserveValidSiblings(t *testing.T) {
	for _, trace := range []bool{false, true} {
		kind, mime, payload := synthetic.ScreenshotCapture, "image/png", png
		if trace {
			kind, mime, payload = synthetic.TraceCapture, "application/zip", []byte("PK\x03\x04fixture")
		}
		t.Run(string(kind), func(t *testing.T) {
			for _, tc := range []struct {
				name  string
				order []string
				want  []string
			}{
				{name: "small first", order: []string{"small", "large"}, want: []string{"small"}},
				{name: "large first", order: []string{"large", "small"}, want: []string{"small"}},
				{name: "several", order: []string{"large", "small", "large2", "small2"}, want: []string{"small", "small2"}},
				{name: "all omitted", order: []string{"large", "large2"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					s, root := store(t)
					work, err := s.Begin(run1, "journey:test", "")
					require.NoError(t, err)
					require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
					label := synthetic.ArtifactTest{ID: "test", Title: "Checkout"}
					var candidates []synthetic.Capture
					for _, id := range tc.order {
						name := "output/" + id + ".png"
						require.NoError(t, os.WriteFile(filepath.Join(work, name), payload, 0600))
						if strings.HasPrefix(id, "large") {
							require.NoError(t, os.Truncate(filepath.Join(work, name), artifacts.FetchMaxBytes+1))
						}
						candidates = append(candidates, synthetic.Capture{ID: id, Kind: kind, MIME: mime, Path: name, Test: label})
					}
					published, err := s.Finalize(t.Context(), run1, candidates)
					require.NoError(t, err)
					var ids []string
					for _, a := range published {
						ids = append(ids, a.ID)
					}
					assert.Equal(t, tc.want, ids)
					for _, dir := range []string{"work", "staging", "drained"} {
						_, err = os.Lstat(filepath.Join(root, dir, run1))
						assert.ErrorIs(t, err, os.ErrNotExist, dir)
					}
					_, err = os.Lstat(work)
					assert.ErrorIs(t, err, os.ErrNotExist, "work alias removed")
					assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
					require.NoError(t, s.Close())
					reopened, err := artifacts.Open(root)
					require.NoError(t, err)
					defer reopened.Close()
					m, err := reopened.Manifest(t.Context(), "journey:test", run1)
					if len(tc.want) == 0 {
						assert.ErrorIs(t, err, artifacts.ErrNotFound)
						_, err = os.Lstat(filepath.Join(root, "runs", run1))
						assert.ErrorIs(t, err, os.ErrNotExist, "no empty manifest")
					} else {
						require.NoError(t, err)
						assert.Equal(t, published, m.Artifacts)
					}
					for _, id := range tc.order {
						a, body, err := reopened.Fetch(t.Context(), "journey:test", run1, id)
						if strings.HasPrefix(id, "large") {
							assert.ErrorIs(t, err, artifacts.ErrNotFound)
						} else {
							require.NoError(t, err)
							assert.Equal(t, payload, body)
							assert.Equal(t, label, a.Test)
						}
					}
				})
			}
		})
	}
}

func TestOversizeDoesNotHideInvalidCaptures(t *testing.T) {
	for _, which := range []string{"large invalid PNG", "large invalid ZIP", "short ZIP", "wrong trace MIME", "duplicate skipped ID", "missing file"} {
		t.Run(which, func(t *testing.T) {
			s, root := store(t)
			work, err := s.Begin(run1, "journey:test", "")
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(work, "output/large.png"), png, 0600))
			require.NoError(t, os.Truncate(filepath.Join(work, "output/large.png"), artifacts.FetchMaxBytes+1))
			require.NoError(t, os.WriteFile(filepath.Join(work, "output/small.png"), png, 0600))
			large := synthetic.Capture{ID: "large", Kind: "screenshot", MIME: "image/png", Path: "output/large.png"}
			small := synthetic.Capture{ID: "small", Kind: "screenshot", MIME: "image/png", Path: "output/small.png"}
			bad := synthetic.Capture{ID: "bad", Kind: "screenshot", MIME: "image/png", Path: "output/missing.png"}
			switch which {
			case "large invalid PNG":
				bad.Path = "output/bad.png"
				require.NoError(t, os.WriteFile(filepath.Join(work, bad.Path), []byte("not a PNG"), 0600))
				require.NoError(t, os.Truncate(filepath.Join(work, bad.Path), artifacts.FetchMaxBytes+1))
			case "large invalid ZIP", "short ZIP", "wrong trace MIME":
				bad.Kind, bad.MIME, bad.Path = "trace", "application/zip", "output/trace.zip"
				body := []byte("PK\x05\x06not a local header")
				if which == "short ZIP" {
					body = []byte("PK")
				}
				if which == "wrong trace MIME" {
					bad.MIME = "image/png"
					body = []byte("PK\x03\x04fixture")
				}
				require.NoError(t, os.WriteFile(filepath.Join(work, bad.Path), body, 0600))
				if which == "large invalid ZIP" {
					require.NoError(t, os.Truncate(filepath.Join(work, bad.Path), artifacts.FetchMaxBytes+1))
				}
			case "duplicate skipped ID":
				bad = large
			}
			published, err := s.Finalize(t.Context(), run1, []synthetic.Capture{large, small, bad})
			require.ErrorIs(t, err, artifacts.ErrInvalidCapture)
			assert.Empty(t, published)
			for _, dir := range []string{"work", "staging", "drained", "runs"} {
				_, err = os.Lstat(filepath.Join(root, dir, run1))
				assert.ErrorIs(t, err, os.ErrNotExist, dir)
			}
			_, err = os.Lstat(work)
			assert.ErrorIs(t, err, os.ErrNotExist)
		})
	}
}

func TestFetchRejectsCorruption(t *testing.T) {
	s, root := store(t)
	_, c := captured(t, s, run2, []byte("good"))
	_, err := s.Finalize(context.Background(), run2, []synthetic.Capture{c})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(root, "runs", run2, c.ID), []byte("bad!"), 0600))
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", run2, c.ID)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest")
}

func TestRetentionAndProtectedWork(t *testing.T) {
	s, root := store(t)
	_, c := captured(t, s, run1, []byte("retained report"))
	_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
	require.NoError(t, err)
	work, err := s.Begin(run2, "lighthouse:test", "")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(work, "profile"), []byte("unverified browser state"), 0600))
	stats, err := s.Enforce(context.Background(), 7, 2<<30)
	require.NoError(t, err)
	assert.Positive(t, *stats.Inventory.Workspace.Bytes)
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	require.NoError(t, err, "fresh capture remains inside policy")
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Stat(work)
	assert.NoError(t, err, "age, budget and restart never prove drainage")
	_, _, err = reopened.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
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
	assert.NotEmpty(t, observedStatus(t, s).Cleanup.Error)
	_, err = os.Stat(filepath.Join(root, "drained", run1))
	require.NoError(t, err)
	// Restore this fixture's alias and retry in the same owner, without restart.
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.Symlink(actual, work))
	_, err = s.Enforce(context.Background(), 7, 1)
	require.NoError(t, err)
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
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
			_, got, err := s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
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
		_, err = s.Begin(id, "lighthouse:test", "")
		assert.ErrorIs(t, err, artifacts.ErrInvalidID)
	}
	_, err = s.Begin(run1, "lighthouse:test", "")
	require.NoError(t, err)
	_, err = s.Begin(run1, "lighthouse:test", "")
	assert.ErrorIs(t, err, artifacts.ErrExists)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.Finalize(ctx, run1, nil)
	assert.ErrorIs(t, err, context.Canceled)
	_, err = s.Finalize(context.Background(), run1, nil)
	require.NoError(t, err)
	_, _, err = s.Fetch(context.Background(), "lighthouse:test", run1, "absent")
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	require.NoError(t, s.Close())
	require.NoError(t, s.Close())
	_, err = s.Begin(run2, "lighthouse:test", "")
	assert.ErrorIs(t, err, artifacts.ErrClosed)
}

func TestTemporaryClosePreservesUnverifiedWork(t *testing.T) {
	s, err := artifacts.Open("")
	require.NoError(t, err)
	work, err := s.Begin(run1, "lighthouse:test", "")
	require.NoError(t, err)
	actual, err := os.Readlink(work)
	require.NoError(t, err)
	require.ErrorContains(t, s.Close(), "temporary artifact store cleanup is incomplete")
	_, err = os.Stat(work)
	require.NoError(t, err)
	// This fixture owns the simulated work; no child process was started.
	require.NoError(t, os.Remove(work))
	require.NoError(t, os.RemoveAll(filepath.Dir(filepath.Dir(actual))))
	s, err = artifacts.Open("")
	require.NoError(t, err)
	work, err = s.Begin(run1, "lighthouse:test", "")
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
	assert.NotEmpty(t, stats.Cleanup.Error)
	assert.Positive(t, *stats.Inventory.Workspace.Bytes)
	_, err = os.Stat(work)
	assert.NoError(t, err)
}

func TestFetchExactSizeBoundary(t *testing.T) {
	for _, trace := range []bool{false, true} {
		name, signature := "report", ""
		if trace {
			name, signature = "trace", "PK\x03\x04"
		}
		t.Run(name, func(t *testing.T) {
			s, _ := store(t)
			payload := append([]byte(signature), []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)-len(signature)))...)
			_, c := captured(t, s, run1, payload)
			if trace {
				c.Kind, c.MIME = "trace", "application/zip"
			}
			_, err := s.Finalize(context.Background(), run1, []synthetic.Capture{c})
			require.NoError(t, err)
			_, got, err := s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
			require.NoError(t, err)
			assert.Equal(t, payload, got)
		})
	}
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
	_, raw, err := s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "long-path report", string(raw))
}

func TestExistingAliasIsNeverAdoptedOrRemoved(t *testing.T) {
	s, root := store(t)
	alias := filepath.Join("/tmp", "nd-dem-"+run1)
	unrelated := t.TempDir()
	require.NoError(t, os.Symlink(unrelated, alias))
	defer os.Remove(alias)
	_, err := s.Begin(run1, "lighthouse:test", "")
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
	_, raw, err := s.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	require.NoError(t, err)
	assert.Equal(t, "candidate", string(raw))
	assert.NotEmpty(t, observedStatus(t, s).Cleanup.Error)
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
	assert.Zero(t, *observedStatus(t, reopened).Inventory.Workspace.Bytes)
	_, raw, err = reopened.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
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
	_, err = s.Manifest(context.Background(), "lighthouse:test", run1)
	require.ErrorContains(t, err, "uncertain")
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	_, err = reopened.Enforce(context.Background(), 7, 2<<30)
	require.NoError(t, err)
	_, err = os.Lstat(work)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, _, err = reopened.Fetch(context.Background(), "lighthouse:test", run1, c.ID)
	assert.ErrorIs(t, err, artifacts.ErrNotFound)
	assert.Zero(t, *observedStatus(t, reopened).Inventory.Workspace.Bytes)
}

// Grow the fixture at the copy's first cancellation check, after its initial Stat.
type growBeforeReadContext struct {
	context.Context
	checks int
	grow   func()
}

func (c *growBeforeReadContext) Err() error {
	c.checks++
	// Admission, drainage and loop entry check cancellation before the copy.
	if c.checks == 4 {
		c.grow()
	}
	return c.Context.Err()
}

func TestCaptureGrowingPastFetchLimitIsNotPublished(t *testing.T) {
	for _, trace := range []bool{false, true} {
		name, signature := "report", ""
		if trace {
			name, signature = "trace", "PK\x03\x04"
		}
		t.Run(name, func(t *testing.T) {
			s, root := store(t)
			work, c := captured(t, s, run1, append([]byte(signature), []byte(strings.Repeat("x", int(artifacts.FetchMaxBytes)-len(signature)))...))
			if trace {
				c.Kind, c.MIME = "trace", "application/zip"
			}
			ctx := &growBeforeReadContext{
				Context: context.Background(),
				grow: func() {
					file, err := os.OpenFile(filepath.Join(work, c.Path), os.O_APPEND|os.O_WRONLY, 0600)
					require.NoError(t, err)
					_, err = file.Write([]byte("extra"))
					require.NoError(t, err)
					require.NoError(t, file.Close())
				},
			}
			_, err := s.Finalize(ctx, run1, []synthetic.Capture{c})
			require.ErrorIs(t, err, artifacts.ErrTooLarge)
			for _, dir := range []string{"work", "staging", "drained", "runs"} {
				_, err = os.Lstat(filepath.Join(root, dir, run1))
				assert.ErrorIs(t, err, os.ErrNotExist, dir)
			}
			assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
		})
	}
}

// Return the admission result, then cancel: a deadline can expire immediately
// after admission while synchronous marker/staging I/O is still running.
type cancelAfterAdmissionContext struct {
	context.Context
	cancel context.CancelFunc
	once   sync.Once
}

func (c *cancelAfterAdmissionContext) Err() error {
	err := c.Context.Err()
	c.once.Do(c.cancel)
	return err
}

func TestCancelledEmptyFinalizeCleansDrainedWorkWithoutPublishing(t *testing.T) {
	s, root := store(t)
	id := fixtureRunID()
	alias, err := s.Begin(id, "lighthouse:test", "")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	admitted := &cancelAfterAdmissionContext{
		Context: ctx,
		cancel:  cancel,
	}
	_, err = s.Finalize(admitted, id, nil)
	require.ErrorIs(t, err, context.Canceled)
	for _, dir := range []string{"work", "staging", "drained", "runs"} {
		_, err := os.Stat(filepath.Join(root, dir, id))
		assert.ErrorIs(t, err, os.ErrNotExist, dir)
	}
	_, err = os.Lstat(alias)
	assert.ErrorIs(t, err, os.ErrNotExist)
	assert.Zero(t, *observedStatus(t, s).Inventory.Workspace.Bytes)
}

func observedStatus(t *testing.T, s *artifacts.Store) artifacts.Status {
	t.Helper()
	page, err := s.Catalogue(context.Background(), artifacts.Filter{
		Limit: 1,
	})
	require.NoError(t, err)
	require.NotNil(t, page.Status.Inventory.Retained.Bytes)
	require.NotNil(t, page.Status.Inventory.Workspace.Bytes)
	return page.Status
}

func TestDiscoveryIgnoresUnpublishedWorkspaceAfterRestart(t *testing.T) {
	s, root := store(t)
	_, err := s.Begin(run1, "journey:test", "")
	require.NoError(t, err)
	assert.False(t, s.HasObservedPublished())
	require.NoError(t, s.Close())
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	assert.False(t, reopened.HasObservedPublished())
	// The earlier workspace remains protected. A separate capture-free completion
	// publishes no set either. These fixtures launch no browser.
	_, err = reopened.Begin(run2, "journey:test", "")
	require.NoError(t, err)
	_, err = reopened.Finalize(t.Context(), run2, nil)
	require.NoError(t, err)
	assert.False(t, reopened.HasObservedPublished())
}

func TestDiscoveryKeepsUnclassifiedPublishedDataVisible(t *testing.T) {
	s, root := store(t)
	require.NoError(t, s.Close())
	require.NoError(t, os.Mkdir(filepath.Join(root, "runs", run1), 0700))
	reopened, err := artifacts.Open(root)
	require.NoError(t, err)
	defer reopened.Close()
	assert.True(t, reopened.HasObservedPublished())
	_, err = reopened.Manifest(t.Context(), "lighthouse:test", run1)
	require.Error(t, err, "discovery must not bypass manifest validation")
}

func TestArtifactAssociationSurvivesRestart(t *testing.T) {
	for _, trace := range []bool{false, true} {
		kind, mime, payload := synthetic.ScreenshotCapture, "image/png", png
		if trace {
			kind, mime, payload = synthetic.TraceCapture, "application/zip", []byte("PK\x03\x04fixture")
		}
		t.Run(string(kind), func(t *testing.T) {
			s, root := store(t)
			work, c := captured(t, s, run1, payload)
			c.Kind, c.MIME = kind, mime
			c.Test = synthetic.ArtifactTest{ID: "test1", Title: "Checkout", TextTruncated: true}
			_, err := s.Finalize(t.Context(), run1, []synthetic.Capture{c})
			require.NoError(t, err)
			_, err = os.Stat(work)
			assert.ErrorIs(t, err, os.ErrNotExist)
			require.NoError(t, s.Close())
			s, err = artifacts.Open(root)
			require.NoError(t, err)
			defer s.Close()
			meta, body, err := s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.NoError(t, err)
			assert.Equal(t, c.Test, meta.Test)
			assert.Equal(t, payload, body)
			filename := filepath.Join(root, "runs", run1, "manifest.json")
			raw, err := os.ReadFile(filename)
			require.NoError(t, err)
			var manifest map[string]any
			require.NoError(t, json.Unmarshal(raw, &manifest))
			assert.Equal(t, float64(2), manifest["version"])
			// Unassociated captures are written without test metadata.
			entry := manifest["artifacts"].([]any)[0].(map[string]any)
			delete(entry, "test")
			raw, err = json.Marshal(manifest)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filename, raw, 0600))
			metadata, _, err := s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.NoError(t, err)
			assert.Empty(t, metadata.Test.ID)
			entry["bytes"] = "corrupt authoritative metadata"
			raw, err = json.Marshal(manifest)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filename, raw, 0600))
			_, _, err = s.Fetch(t.Context(), "lighthouse:test", run1, c.ID)
			require.Error(t, err)
		})
	}
}

func TestCatalogueSnapshotsSiteWithoutChangingOwnership(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	s, err := artifacts.Open(root)
	require.NoError(t, err)
	ids := []string{fixtureRunID(), fixtureRunID(), fixtureRunID()}
	for i, site := range []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", ""} {
		work, err := s.Begin(ids[i], "lighthouse:home", site)
		require.NoError(t, err)
		require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(work, "output/report"), []byte("<html>report</html>"), 0600))
		_, err = s.Finalize(ctx, ids[i], []synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report"}})
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())
	s, err = artifacts.Open(root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	filtered, err := s.Catalogue(ctx, artifacts.Filter{VnodeGUID: "11111111-1111-4111-8111-111111111111", JobID: "lighthouse:home", Limit: 1})
	require.NoError(t, err)
	require.Len(t, filtered.Sets, 1)
	assert.Equal(t, ids[0], filtered.Sets[0].RunID)
	assert.Equal(t, "11111111-1111-4111-8111-111111111111", filtered.Sets[0].VnodeGUID)
	mismatch, err := s.Catalogue(ctx, artifacts.Filter{VnodeGUID: "11111111-1111-4111-8111-111111111111", JobID: "journey:home", Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, mismatch.Sets)
	all, err := s.Catalogue(ctx, artifacts.Filter{Limit: 10})
	require.NoError(t, err)
	require.Len(t, all.Sets, 3)
	assert.Equal(t, 3, all.Status.Inventory.KnownSets)
	assert.Equal(t, 1, all.Status.Inventory.KnownJobs)
	manifest, err := s.Manifest(ctx, "lighthouse:home", ids[2])
	require.NoError(t, err)
	assert.Empty(t, manifest.VnodeGUID)
	_, _, err = s.Fetch(ctx, "journey:home", ids[0], "report")
	assert.ErrorIs(t, err, artifacts.ErrOwnerMismatch)
	_, body, err := s.Fetch(ctx, "lighthouse:home", ids[0], "report")
	require.NoError(t, err)
	assert.Equal(t, "<html>report</html>", string(body))
}
