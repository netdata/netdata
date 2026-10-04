// SPDX-License-Identifier: GPL-3.0-or-later
package synthetic_test

import (
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestGenerationSnapshotsAndFreshness(t *testing.T) {
	h := synthetic.NewHub()
	now := time.Now()
	job := synthetic.Job{JobID: "journey:shop", CadenceSeconds: 900, TimeoutSeconds: 120}
	reg, err := h.Register(job)
	require.NoError(t, err)
	initial := h.Snapshot(now)
	require.Len(t, initial, 1)
	assert.Equal(t, "unknown", initial[0].State)
	assert.Nil(t, initial[0].Latest)
	_, err = h.Register(job)
	require.Error(t, err)
	duration := 1.0
	run := synthetic.Run{ID: "id", JobID: job.JobID, Outcome: synthetic.Failed, CompletedUS: now.UnixMicro(), Tests: &synthetic.TestCounts{Declared: 1, Failed: 1}, Events: []synthetic.Event{{Message: "original", DurationMS: &duration}}}
	reg.Complete(run)
	run.Tests.Failed = 0
	run.Events[0].Message = "changed"
	duration = 3
	snap := h.Snapshot(now.Add(time.Second))
	assert.True(t, snap[0].Fresh)
	assert.Equal(t, 1, snap[0].Latest.Tests.Failed)
	assert.Equal(t, "original", snap[0].Latest.Events[0].Message)
	assert.Equal(t, 1.0, *snap[0].Latest.Events[0].DurationMS)
	snap[0].Latest.Tests.Failed = 0
	assert.Equal(t, 1, h.Snapshot(now)[0].Latest.Tests.Failed)
	reg.SetState("running")
	old := h.Snapshot(now.Add(1801 * time.Second))
	assert.Equal(t, "running", old[0].State)
	assert.False(t, old[0].Fresh)
	reg.Retire()
	require.Empty(t, h.Snapshot(now))
	replacement, err := h.Register(job)
	require.NoError(t, err)
	reg.Complete(run)
	reg.Retire()
	assert.Nil(t, h.Snapshot(now)[0].Latest)
	replacement.Retire()
}
