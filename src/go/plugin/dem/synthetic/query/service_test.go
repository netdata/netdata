// SPDX-License-Identifier: GPL-3.0-or-later
package query_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
	service := query.New(active, retained, nil)
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

func TestEvidenceRequiresRecordedMembershipAndPreservesExpiredMetadata(t *testing.T) {
	ctx := context.Background()
	captures, err := artifacts.Open(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, captures.Close()) })
	const runID = "0123456789abcdef0123456789abcdef"
	work, err := captures.Begin(runID)
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
	service := query.New(active, nil, captures)
	// The file really exists, but this observation did not record it. Disk presence
	// alone must never grant access to bytes omitted from the exact run's evidence.
	evidence, err := service.Evidence(ctx, run.JobID, run.ID, "report")
	assert.ErrorIs(t, err, query.ErrArtifactNotRecorded)
	assert.Empty(t, evidence.Body)
	run.Artifacts = manifest
	registration.Complete(run)
	evidence, err = service.Evidence(ctx, run.JobID, run.ID, "report")
	require.NoError(t, err)
	assert.Equal(t, content, evidence.Body)
	assert.Equal(t, "available", evidence.CaptureState)
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
