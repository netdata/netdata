// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
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

func TestRunUsesCopiedActiveEvidenceThenRetainedHistory(t *testing.T) {
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
	active := registry.New()
	registration, err := active.Register(registry.Job{
		JobID:          run.JobID,
		CadenceSeconds: 900,
	})
	require.NoError(t, err)
	run.HistoryError = "completion append outcome uncertain"
	run.Events[0].Message = "latest detail"
	registration.Complete(run)
	service := query.New(active, retained, nil, nil)
	got, err := service.Run(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run, got)
	got.Events[0].Message = "caller edit"
	again, err := service.Run(ctx, run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, "latest detail", again.Events[0].Message)
	registration.Retire()
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
	const runID = "0123456789abcdef0123456789abcdef"
	work, err := captures.Begin(runID, "lighthouse:home")
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
	active := registry.New()
	run := synthetic.Run{
		ID:           runID,
		JobID:        "lighthouse:home",
		CaptureState: "available",
		CompletedUS:  time.Now().UnixMicro(),
	}
	registration, err := active.Register(registry.Job{
		JobID:          run.JobID,
		CadenceSeconds: 900,
	})
	require.NoError(t, err)
	defer registration.Retire()
	registration.Complete(run)
	service := query.New(active, nil, captures, nil)
	// The manifest owns retained bytes even when the active observation does not
	// mention them. Arbitrary files and wrong owners never grant membership.
	evidence, err := service.Evidence(ctx, run.JobID, run.ID, "unrecorded")
	assert.ErrorIs(t, err, query.ErrArtifactNotRecorded)
	assert.Empty(t, evidence.Body)
	evidence, err = service.Evidence(ctx, run.JobID, run.ID, "report")
	require.NoError(t, err)
	assert.Equal(t, content, evidence.Body)
	assert.Equal(t, "available", evidence.CaptureState)
	run.Artifacts = manifest
	registration.Complete(run)
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
	assert.Equal(t, "expired_or_unavailable", evidence.CaptureState)
	assert.Empty(t, evidence.Body)
}

func TestRetainedCapturesOutliveHistoryAndActiveJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		captures, err := artifacts.Open(root)
		require.NoError(t, err)
		const runID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		const jobID = "lighthouse:retired"
		work, err := captures.Begin(runID, jobID)
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
		service := query.New(nil, retained, captures, func() string { return "invalid policy reload" })
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

func TestInvalidManifestAndWrongOwnerNeverFallBackToHistory(t *testing.T) {
	for _, damage := range []string{"corrupt", "missing", "wrong owner"} {
		t.Run(damage, func(t *testing.T) {
			ctx := context.Background()
			root := t.TempDir()
			captures, err := artifacts.Open(root)
			require.NoError(t, err)
			defer captures.Close()
			const runID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			work, err := captures.Begin(runID, "lighthouse:owner")
			require.NoError(t, err)
			require.NoError(t, os.Mkdir(filepath.Join(work, "output"), 0700))
			require.NoError(t, os.WriteFile(filepath.Join(work, "output/report.html"), []byte("report"), 0600))
			entries, err := captures.Finalize(
				ctx,
				runID,
				[]synthetic.Capture{{ID: "report", Kind: "report", MIME: "text/html", Path: "output/report.html"}},
			)
			require.NoError(t, err)
			active := registry.New()
			jobID := "lighthouse:owner"
			if damage == "wrong owner" {
				jobID = "lighthouse:impostor"
			}
			registration, err := active.Register(registry.Job{
				JobID:          jobID,
				CadenceSeconds: 900,
			})
			require.NoError(t, err)
			defer registration.Retire()
			registration.Complete(synthetic.Run{
				ID:           runID,
				JobID:        jobID,
				Artifacts:    entries,
				CaptureState: "available",
			})
			manifestPath := filepath.Join(root, "runs", runID, "manifest.json")
			switch damage {
			case "corrupt":
				require.NoError(t, os.WriteFile(manifestPath, []byte("broken"), 0600))
			case "missing":
				require.NoError(t, os.Remove(manifestPath))
			}
			service := query.New(active, nil, captures, nil)
			for _, id := range []string{"", "report"} {
				result, err := service.Evidence(ctx, jobID, runID, id)
				require.Error(t, err)
				assert.Empty(t, result.Body)
				assert.NotEqual(t, "expired_or_unavailable", result.CaptureState)
				if damage != "wrong owner" {
					assert.NotErrorIs(t, err, query.ErrNotFound)
				}
			}
		})
	}
}
