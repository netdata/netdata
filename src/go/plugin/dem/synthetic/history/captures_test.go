// SPDX-License-Identifier: GPL-3.0-or-later
package history_test

import (
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/history"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaptureObservationsSurviveHistoryRestart(t *testing.T) {
	root := t.TempDir()
	j, err := journal.Open(t.Context(), root)
	require.NoError(t, err)
	test := synthetic.ArtifactTest{ID: "t1", Title: "Checkout"}
	run := synthetic.Run{ID: "capture-run", JobID: "journey:test", Kind: synthetic.Journey, Name: "test", StartedUS: time.Now().UnixMicro(), CompletedUS: time.Now().UnixMicro(), Outcome: synthetic.Failed, CaptureState: "unavailable",
		Captures: &synthetic.CaptureReport{Reporting: "complete", Trace: "supported", Tests: []synthetic.CaptureTest{{ArtifactTest: test, Status: "failed", ExpectedStatus: "passed", Accepted: 1, Unaccepted: 1}}, Associations: []synthetic.CaptureAssociation{{ID: "s1", TestID: "t1"}, {ID: "trace1", TestID: "t1"}}, Unpublished: []synthetic.CaptureOmission{{ID: "s1", Reason: "too_large"}}},
	}
	_, err = history.NewStore(j).AppendRun(t.Context(), "complete", run)
	require.NoError(t, err)
	require.NoError(t, j.Close())
	j, err = journal.Open(t.Context(), root)
	require.NoError(t, err)
	defer j.Close()
	got, err := history.NewStore(j).GetRun(t.Context(), run.JobID, run.ID)
	require.NoError(t, err)
	assert.Equal(t, run.Captures, got.Captures)
}
