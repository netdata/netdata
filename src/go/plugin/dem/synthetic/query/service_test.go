// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/query"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fixtureRunID returns a fresh run ID: artifacts.Store.Begin creates a /tmp/nd-dem-<id> alias that is shared by
// every test process on the host, so fixed IDs collide across parallel packages and concurrent checkouts.
func fixtureRunID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}

// activeRuns serves the latest run of each active job, keyed by job ID.
type activeRuns map[string]synthetic.Run

func (a activeRuns) Snapshot(time.Time) []registry.Job { return nil }

func (a activeRuns) Latest(jobID, runID string) (synthetic.Run, bool) {
	run, ok := a[jobID]
	if !ok || run.ID != runID {
		return synthetic.Run{}, false
	}
	return run, true
}

func TestRunUsesActiveEvidenceThenRetainedHistory(t *testing.T) {
	ctx := context.Background()
	log, err := journal.Open(ctx, t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, log.Close()) })
	retained := history.NewStore(log)
	run := synthetic.Run{
		ID:        "run",
		JobID:     "journey:checkout",
		Kind:      synthetic.Journey,
		StartedUS: time.Now().UnixMicro(),
		Outcome:   synthetic.Failed,
		Events:    []synthetic.Event{{Message: "retained detail"}},
	}
	run.CompletedUS = run.StartedUS + 1
	_, err = retained.AppendRun(ctx, "complete", run)
	require.NoError(t, err)
	latest := run
	latest.HistoryError = "completion append outcome uncertain"
	latest.Events = []synthetic.Event{{Message: "latest detail"}}
	active := activeRuns{run.JobID: latest}
	service := query.New(active, retained, emptyArtifacts(t), noPolicyError)
	got, err := service.Run(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, latest, got)
	delete(active, run.JobID)
	got, err = service.Run(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Empty(t, got.HistoryError)
	assert.Equal(t, "retained detail", got.Events[0].Message)
	_, err = service.Run(ctx, "journey:another", run.ID)
	assert.ErrorIs(t, err, query.ErrNotFound)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = service.Run(cancelled, run.JobID, run.ID)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestEvidenceUsesManifestMembershipAndPreservesExpiredMetadata(t *testing.T) {
	ctx := context.Background()
	captures, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	runID := fixtureRunID()
	work, err := captures.Begin(runID, "lighthouse:home", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	content := []byte("<html>private report</html>")
	require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), content, 0600))
	manifest, err := captures.Finalize(
		ctx,
		runID,
		[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
	)
	require.NoError(t, err)
	run := synthetic.Run{
		ID:           runID,
		JobID:        "lighthouse:home",
		CaptureState: "available",
		CompletedUS:  time.Now().UnixMicro(),
	}
	active := activeRuns{run.JobID: run}
	service := query.New(active, emptyHistory(t), captures, noPolicyError)
	// The manifest owns retained bytes even when the active observation does not
	// mention them. Arbitrary files and wrong owners never grant membership.
	evidence, err := service.Evidence(ctx, run.JobID, run.ID, "unrecorded")
	assert.ErrorIs(t, err, query.ErrArtifactNotRecorded)
	assert.Empty(t, evidence.Body)
	evidence, err = service.Evidence(ctx, run.JobID, run.ID, "report")
	require.NoError(t, err)
	assert.Equal(t, content, evidence.Body)
	assert.Equal(t, synthetic.CaptureAvailable, evidence.CaptureState)
	run.Artifacts = manifest
	active[run.JobID] = run
	_, err = captures.Enforce(ctx, 7, 1)
	require.NoError(t, err)
	evidence, err = service.Evidence(ctx, run.JobID, run.ID, "")
	require.NoError(t, err)
	require.Len(t, evidence.Artifacts, 1)
	assert.Equal(t, manifest[0], evidence.Artifacts[0].Artifact)
	assert.Equal(t, "expired_or_unavailable", evidence.Artifacts[0].Availability)
	evidence, err = service.Evidence(ctx, run.JobID, run.ID, "report")
	assert.ErrorIs(t, err, query.ErrExpired)
	assert.Equal(t, run.ID, evidence.RunID)
	assert.Equal(t, run.JobID, evidence.JobID)
	assert.Equal(t, synthetic.CaptureExpired, evidence.CaptureState)
	assert.Empty(t, evidence.Body)
}

func TestRetainedCapturesOutliveHistoryAndActiveJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		captures, err := artifacts.Open(root)
		require.NoError(t, err)
		runID := fixtureRunID()
		const jobID = "lighthouse:retired"
		work, err := captures.Begin(runID, jobID, "")
		require.NoError(t, err)
		body := []byte("<html>retained report</html>")
		require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
		require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), body, 0600))
		entries, err := captures.Finalize(
			ctx,
			runID,
			[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
		)
		require.NoError(t, err)
		log, err := journal.Open(ctx, t.TempDir())
		require.NoError(t, err)
		defer log.Close()
		retained := history.NewStore(log)
		now := time.Now().UnixMicro()
		_, err = retained.AppendRun(
			ctx,
			"complete",
			synthetic.Run{
				ID:           runID,
				JobID:        jobID,
				Kind:         synthetic.Lighthouse,
				StartedUS:    now,
				CompletedUS:  now + 1,
				Outcome:      synthetic.Success,
				CaptureState: "available",
				Artifacts:    entries,
			},
		)
		require.NoError(t, err)
		require.NoError(t, retained.Sync(ctx))
		_, err = retained.GetRun(ctx, jobID, runID)
		require.NoError(t, err)
		time.Sleep(3 * 24 * time.Hour)
		require.NoError(t, log.EnforceHistoryRetention(ctx, 1, journal.MinHistoryBytes))
		_, err = retained.GetRun(ctx, jobID, runID)
		require.ErrorIs(t, err, os.ErrNotExist)
		require.NoError(t, captures.Close())
		captures, err = artifacts.Open(root)
		require.NoError(t, err)
		defer captures.Close()
		service := query.New(activeRuns{}, retained, captures, func() string { return "invalid policy reload" })
		page, err := service.Catalogue(ctx, artifacts.Filter{
			Limit: 1,
		})
		require.NoError(t, err)
		require.Len(t, page.Sets, 1)
		assert.Equal(t, runID, page.Sets[0].RunID)
		assert.Equal(t, jobID, page.Sets[0].JobID)
		assert.Equal(t, "invalid policy reload", page.PolicyError)
		evidence, err := service.Evidence(ctx, jobID, runID, "report")
		require.NoError(t, err)
		assert.Equal(t, body, evidence.Body)
		_, err = service.Evidence(ctx, "lighthouse:other", runID, "report")
		assert.ErrorIs(t, err, query.ErrNotFound)
	})
}

func TestScreenshotAssociationWithoutHistoryOrActiveJob(t *testing.T) {
	root := t.TempDir()
	store, err := artifacts.Open(root)
	require.NoError(t, err)
	id := fixtureRunID()
	work, err := store.Begin(id, "journey:retired", "")
	require.NoError(t, err)
	require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
	body := []byte("\x89PNG\r\n\x1a\nfixture")
	require.NoError(t, os.WriteFile(filepath.Join(work, "output/screenshot.png"), body, 0600))
	test := synthetic.ArtifactTest{ID: "test1", Title: "Checkout"}
	_, err = store.Finalize(t.Context(), id, []synthetic.Capture{{ID: "s1", Kind: "screenshot", MIME: "image/png", Path: "output/screenshot.png", Test: test}})
	require.NoError(t, err)
	require.NoError(t, store.Close())
	store, err = artifacts.Open(root)
	require.NoError(t, err)
	defer store.Close()
	service := query.New(activeRuns{}, emptyHistory(t), store, noPolicyError)
	evidence, err := service.Evidence(t.Context(), "journey:retired", id, "s1")
	require.NoError(t, err)
	require.Len(t, evidence.Artifacts, 1)
	assert.Equal(t, test, evidence.Artifacts[0].Test)
	assert.Equal(t, body, evidence.Body)
	_, err = service.Evidence(t.Context(), "journey:other", id, "s1")
	require.ErrorIs(t, err, query.ErrNotFound)
}

func TestInvalidManifestAndWrongOwnerNeverFallBackToHistory(t *testing.T) {
	for _, damage := range []string{"corrupt", "missing", "wrong owner"} {
		t.Run(damage, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			captures, err := artifacts.Open(root)
			require.NoError(t, err)
			defer captures.Close()
			runID := fixtureRunID()
			work, err := captures.Begin(runID, "lighthouse:owner", "")
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), []byte("report"), 0600))
			entries, err := captures.Finalize(
				ctx,
				runID,
				[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
			)
			require.NoError(t, err)
			jobID := "lighthouse:owner"
			if damage == "wrong owner" {
				jobID = "lighthouse:impostor"
			}
			active := activeRuns{jobID: {
				ID:           runID,
				JobID:        jobID,
				Artifacts:    entries,
				CaptureState: "available",
			}}
			manifestPath := filepath.Join(root, "runs", runID, "manifest.json")
			switch damage {
			case "corrupt":
				require.NoError(t, os.WriteFile(manifestPath, []byte("broken"), 0600))
			case "missing":
				require.NoError(t, os.Remove(manifestPath))
			}
			service := query.New(active, unreadHistory{t}, captures, noPolicyError)
			for _, id := range []string{"", "report"} {
				result, err := service.Evidence(ctx, jobID, runID, id)
				require.Error(t, err)
				assert.Empty(t, result.Body)
				assert.NotEqual(t, synthetic.CaptureExpired, result.CaptureState)
				if damage != "wrong owner" {
					assert.NotErrorIs(t, err, query.ErrNotFound)
				}
			}
		})
	}
}

// emptyHistory is a real run history holding no runs.
func emptyHistory(t *testing.T) *history.Store {
	t.Helper()
	store, err := journal.Open(t.Context(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return history.NewStore(store)
}

// emptyArtifacts is a real capture store holding no captures.
func emptyArtifacts(t *testing.T) *artifacts.Store {
	t.Helper()
	store, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	return store
}

func noPolicyError() string { return "" }

// unreadHistory fails the test when an evidence read falls back to run history.
type unreadHistory struct{ t *testing.T }

func (h unreadHistory) QueryRuns(context.Context, history.RunFilter) (history.RunPage, error) {
	h.t.Error("run history must not be queried")
	return history.RunPage{}, nil
}

func (h unreadHistory) GetRun(context.Context, string, string) (synthetic.Run, error) {
	h.t.Error("run history must not be read")
	return synthetic.Run{}, os.ErrNotExist
}
