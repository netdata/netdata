// SPDX-License-Identifier: GPL-3.0-or-later
package artifacts_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type retentionFixture struct {
	t     *testing.T
	store *artifacts.Store
	root  string
	ids   []string
}

func newRetentionFixture(t *testing.T) *retentionFixture {
	t.Helper()
	f := &retentionFixture{
		t:    t,
		root: filepath.Join(t.TempDir(), "captures"),
	}
	var err error
	f.store, err = artifacts.Open(f.root)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, f.store.Close())
		// No fixture launches children. Remove only aliases still owned by it.
		for _, id := range f.ids {
			alias := filepath.Join("/tmp", "nd-dem-"+id)
			if target, err := os.Readlink(alias); err == nil && target == filepath.Join(f.root, "work", id) {
				require.NoError(t, os.Remove(alias))
			}
		}
	})
	return f
}

func (f *retentionFixture) begin(job string) (string, string) {
	f.t.Helper()
	id := fixtureRunID()
	work, err := f.store.Begin(id, job)
	require.NoError(f.t, err)
	f.ids = append(f.ids, id)
	return id, work
}

func (f *retentionFixture) publish(job, kind string, bytes int) string {
	f.t.Helper()
	id, work := f.begin(job)
	payload := []byte(strings.Repeat("x", bytes))
	mime := "text/html"
	if kind == "screenshot" {
		payload = append([]byte("\x89PNG\r\n\x1a\n"), payload...)
		mime = "image/png"
	}
	require.NoError(f.t, os.Mkdir(filepath.Join(work, "output"), 0700))
	require.NoError(f.t, os.WriteFile(filepath.Join(work, "output", "capture"), payload, 0600))
	out, err := f.store.Finalize(context.Background(), id, []synthetic.Capture{{
		ID: "capture", Kind: kind, MIME: mime, Path: "output/capture",
	}})
	require.NoError(f.t, err)
	require.Len(f.t, out, 1)
	return id
}

func (f *retentionFixture) size(id string) int64 {
	f.t.Helper()
	var size int64
	err := filepath.WalkDir(filepath.Join(f.root, "runs", id), func(_ string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err == nil {
			size += info.Size()
		}
		return err
	})
	require.NoError(f.t, err)
	return size
}

func (f *retentionFixture) retained(job, id string, want bool) {
	f.t.Helper()
	manifest, err := f.store.Manifest(context.Background(), job, id)
	if !want {
		assert.ErrorIs(f.t, err, artifacts.ErrNotFound)
		_, _, err = f.store.Fetch(context.Background(), job, id, "capture")
		assert.ErrorIs(f.t, err, artifacts.ErrNotFound)
		return
	}
	require.NoError(f.t, err)
	assert.Equal(f.t, job, manifest.JobID)
	assert.Equal(f.t, id, manifest.RunID)
	require.Len(f.t, manifest.Artifacts, 1)
	_, payload, err := f.store.Fetch(context.Background(), job, id, "capture")
	require.NoError(f.t, err)
	assert.NotEmpty(f.t, payload)
}

func TestRetentionPrefersNewestCapturePerJob(t *testing.T) {
	for _, test := range []struct{ name, quietJob, kind string }{
		{"mixed capture kinds", "journey:quiet", "screenshot"},
		{"same capture kind", "lighthouse:quiet", "report"},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newRetentionFixture(t)
				quiet := f.publish(test.quietJob, test.kind, 64)
				time.Sleep(time.Second)
				old := f.publish("lighthouse:noisy", "report", 64)
				time.Sleep(time.Second)
				latest := f.publish("lighthouse:noisy", "report", 64)
				budget := f.size(quiet) + f.size(latest)
				status, err := f.store.Enforce(context.Background(), 7, budget)
				require.NoError(t, err)
				f.retained(test.quietJob, quiet, true)
				f.retained("lighthouse:noisy", old, false)
				f.retained("lighthouse:noisy", latest, true)
				require.NotNil(t, status.Inventory.Retained.Bytes)
				assert.Equal(t, budget, *status.Inventory.Retained.Bytes)
				assert.Equal(t, 2, status.Inventory.KnownJobs)
			})
		})
	}
}

func TestRetentionRemovesExpiredAndOversizedBeforeSelectingFallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		expired := f.publish("lighthouse:expired", "report", 64)
		time.Sleep(8 * 24 * time.Hour)
		fallback := f.publish("lighthouse:changing", "report", 64)
		time.Sleep(time.Second)
		other := f.publish("journey:quiet", "screenshot", 64)
		time.Sleep(time.Second)
		oversized := f.publish("lighthouse:changing", "report", 8192)
		budget := f.size(fallback) + f.size(other)
		require.Greater(t, f.size(oversized), budget)
		_, err := f.store.Enforce(context.Background(), 7, budget)
		require.NoError(t, err)
		f.retained("lighthouse:expired", expired, false)
		f.retained("lighthouse:changing", oversized, false)
		f.retained("lighthouse:changing", fallback, true)
		f.retained("journey:quiet", other, true)
	})
}

func TestRetentionOversubscribedPreferencesLoseOldestJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		oldest := f.publish("lighthouse:old", "report", 64)
		time.Sleep(time.Second)
		middle := f.publish("lighthouse:middle", "report", 64)
		time.Sleep(time.Second)
		newest := f.publish("journey:new", "screenshot", 64)
		_, err := f.store.Enforce(context.Background(), 7, f.size(middle)+f.size(newest))
		require.NoError(t, err)
		f.retained("lighthouse:old", oldest, false)
		f.retained("lighthouse:middle", middle, true)
		f.retained("journey:new", newest, true)
	})
}

func TestRetentionPublicationTiesUseRunID(t *testing.T) {
	for _, sameJob := range []bool{false, true} {
		name := "across jobs"
		if sameJob {
			name = "within job"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				f := newRetentionFixture(t)
				jobA, jobB := "lighthouse:a", "lighthouse:b"
				if sameJob {
					jobB = jobA
				}
				a := f.publish(jobA, "report", 64)
				b := f.publish(jobB, "report", 64)
				ma, err := f.store.Manifest(context.Background(), jobA, a)
				require.NoError(t, err)
				mb, err := f.store.Manifest(context.Background(), jobB, b)
				require.NoError(t, err)
				require.Equal(t, ma.PublishedUS, mb.PublishedUS)
				if a > b {
					a, b, jobA, jobB = b, a, jobB, jobA
				}
				_, err = f.store.Enforce(context.Background(), 7, f.size(b))
				require.NoError(t, err)
				f.retained(jobA, a, false)
				f.retained(jobB, b, true)
			})
		})
	}
}

func TestRetentionWorkspaceDoesNotDisplaceCaptureAfterRestart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		capture := f.publish("journey:failure", "screenshot", 64)
		budget := f.size(capture)
		_, work := f.begin("journey:in-flight")
		payload := []byte(strings.Repeat("private browser state", 1000))
		require.NoError(t, os.WriteFile(filepath.Join(work, "profile"), payload, 0600))
		for _, restart := range []bool{false, true} {
			if restart {
				require.NoError(t, f.store.Close())
				var err error
				f.store, err = artifacts.Open(f.root)
				require.NoError(t, err)
			}
			status, err := f.store.Enforce(context.Background(), 7, budget)
			require.NoError(t, err)
			f.retained("journey:failure", capture, true)
			require.NotNil(t, status.Inventory.Workspace.Bytes)
			assert.GreaterOrEqual(t, *status.Inventory.Workspace.Bytes, int64(len(payload)))
			got, err := os.ReadFile(filepath.Join(work, "profile"))
			require.NoError(t, err)
			assert.Equal(t, payload, got)
		}
	})
}

func TestNoCaptureDoesNotPublishOrReplaceFailureCapture(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		failure := f.publish("journey:check", "screenshot", 64)
		time.Sleep(time.Second)
		healthy, work := f.begin("journey:check")
		out, err := f.store.Finalize(context.Background(), healthy, nil)
		require.NoError(t, err)
		assert.Empty(t, out)
		for _, path := range []string{work, filepath.Join(f.root, "runs", healthy)} {
			_, err := os.Lstat(path)
			assert.ErrorIs(t, err, os.ErrNotExist)
		}
		_, err = f.store.Enforce(context.Background(), 7, f.size(failure))
		require.NoError(t, err)
		f.retained("journey:check", failure, true)
		f.retained("journey:check", healthy, false)
		page, err := f.store.Catalogue(context.Background(), artifacts.Filter{
			Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, page.Sets, 1)
		assert.Equal(t, failure, page.Sets[0].RunID)
	})
}

func TestCataloguePaginationFiltersAndGlobalInventory(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		old := f.publish("journey:a", "screenshot", 64)
		time.Sleep(time.Second)
		one := f.publish("lighthouse:b", "report", 64)
		two := f.publish("lighthouse:b", "report", 64)
		expected := []string{one, two}
		slices.Sort(expected)
		slices.Reverse(expected)
		expected = append(expected, old)
		total := f.size(old) + f.size(one) + f.size(two)
		var cursor *artifacts.Cursor
		var got []string
		for range 3 {
			page, err := f.store.Catalogue(context.Background(), artifacts.Filter{
				Limit:  1,
				Before: cursor,
			})
			require.NoError(t, err)
			require.Len(t, page.Sets, 1)
			got = append(got, page.Sets[0].RunID)
			assert.Equal(t, f.size(page.Sets[0].RunID), page.Sets[0].Bytes)
			assert.Equal(t, 1, page.Sets[0].Artifacts)
			require.NotNil(t, page.Status.Inventory.Retained.Bytes)
			assert.Equal(t, total, *page.Status.Inventory.Retained.Bytes)
			assert.Equal(t, 3, page.Status.Inventory.KnownSets)
			assert.Equal(t, 2, page.Status.Inventory.KnownJobs)
			cursor = page.Next
			if len(got) < 3 {
				require.NotNil(t, cursor)
			}
		}
		assert.Nil(t, cursor)
		assert.Equal(t, expected, got)
		for _, job := range []string{"journey:a", "journey:absent"} {
			page, err := f.store.Catalogue(context.Background(), artifacts.Filter{
				JobID: job,
				Limit: 10,
			})
			require.NoError(t, err)
			if job == "journey:a" {
				require.Len(t, page.Sets, 1)
				assert.Equal(t, old, page.Sets[0].RunID)
			} else {
				assert.Empty(t, page.Sets)
			}
			require.NotNil(t, page.Status.Inventory.Retained.Bytes)
			assert.Equal(t, total, *page.Status.Inventory.Retained.Bytes)
			assert.Equal(t, 3, page.Status.Inventory.KnownSets)
			assert.Equal(t, 2, page.Status.Inventory.KnownJobs)
		}
		_, err := f.store.Manifest(context.Background(), "journey:wrong", old)
		assert.ErrorIs(t, err, artifacts.ErrOwnerMismatch)
		_, _, err = f.store.Fetch(context.Background(), "journey:wrong", old, "capture")
		assert.ErrorIs(t, err, artifacts.ErrOwnerMismatch)
	})
}

func TestIncompleteWorkspaceInventoryDoesNotBlockPublishedRetention(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		old := f.publish("lighthouse:check", "report", 64)
		time.Sleep(time.Second)
		latest := f.publish("lighthouse:check", "report", 64)
		budget := f.size(latest)
		_, work := f.begin("journey:in-flight")
		require.NoError(t, os.WriteFile(filepath.Join(work, "profile"), []byte("private state"), 0600))
		actual, err := os.Readlink(work)
		require.NoError(t, err)
		denyDirectoryReads(t, actual)
		status, err := f.store.Enforce(context.Background(), 7, budget)
		require.Error(t, err)
		assert.Nil(t, status.Inventory.Workspace.Bytes, "partial bytes must not become a complete observation")
		assert.NotEmpty(t, status.Inventory.Workspace.Error)
		require.NotNil(t, status.Inventory.Retained.Bytes)
		assert.Equal(t, budget, *status.Inventory.Retained.Bytes)
		f.retained("lighthouse:check", old, false)
		f.retained("lighthouse:check", latest, true)
		page, err := f.store.Catalogue(context.Background(), artifacts.Filter{
			Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, page.Sets, 1)
		assert.Equal(t, latest, page.Sets[0].RunID)
		assert.Nil(t, page.Status.Inventory.Workspace.Bytes)
		assert.NotEmpty(t, page.Status.Inventory.Workspace.Error)
		require.NoError(t, os.Chmod(actual, 0700))
		payload, err := os.ReadFile(filepath.Join(work, "profile"))
		require.NoError(t, err)
		assert.Equal(t, "private state", string(payload))
	})
}

func TestCorruptPublishedManifestRemainsUnclassifiedAndPreserved(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newRetentionFixture(t)
		unknown := f.publish("journey:unknown", "screenshot", 64)
		manifestPath := filepath.Join(f.root, "runs", unknown, "manifest.json")
		// Corrupt a genuinely published set to exercise incomplete on-disk evidence.
		require.NoError(t, os.WriteFile(manifestPath, []byte("{broken"), 0600))
		unknownBytes := f.size(unknown)
		time.Sleep(time.Second)
		old := f.publish("lighthouse:check", "report", 64)
		time.Sleep(time.Second)
		latest := f.publish("lighthouse:check", "report", 64)
		budget := f.size(latest)
		require.NoError(t, f.store.Close())
		var err error
		f.store, err = artifacts.Open(f.root)
		require.NoError(t, err, "incomplete inventory must not prevent opening a healthy owned root")
		status, err := f.store.Enforce(context.Background(), 7, budget)
		require.Error(t, err)
		assert.NotEmpty(t, status.Inventory.Error)
		require.NotNil(t, status.Inventory.Unclassified.Bytes)
		assert.Equal(t, unknownBytes, *status.Inventory.Unclassified.Bytes)
		require.NotNil(t, status.Inventory.Retained.Bytes)
		assert.Equal(t, budget, *status.Inventory.Retained.Bytes)
		f.retained("lighthouse:check", old, false)
		f.retained("lighthouse:check", latest, true)
		raw, err := os.ReadFile(manifestPath)
		require.NoError(t, err)
		assert.Equal(t, "{broken", string(raw))
		_, err = f.store.Manifest(context.Background(), "journey:unknown", unknown)
		require.Error(t, err)
		assert.NotErrorIs(t, err, artifacts.ErrNotFound, "corruption must not be reported as ordinary absence")
		page, err := f.store.Catalogue(context.Background(), artifacts.Filter{
			Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, page.Sets, 1)
		assert.Equal(t, latest, page.Sets[0].RunID)
		assert.NotEmpty(t, page.Status.Inventory.Error)
		assert.Equal(t, 1, page.Status.Inventory.KnownSets)
	})
}
