//go:build linux || darwin || freebsd

// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualificationViewSharesAdmissionAndKeepsMonitoringSource(t *testing.T) {
	e, history, artifacts := testEngine(t)
	started, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		if calls.Add(1) == 1 {
			close(started)
			<-release
		}
		return resultPayload(success())(ctx, args, raw, out, stderr)
	})
	paths := e.source.(fixtureRuntime).Paths
	paths.Identity.Version = "candidate-b"
	paths.Identity.SHA256 = strings.Repeat("b", 64)
	candidate := e.qualificationView(fixtureRuntime{Paths: paths}, artifacts)
	done := make(chan synthetic.Execution, 1)
	go func() { done <- e.Execute(context.Background(), journey(), nil) }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	queued := candidate.Execute(ctx, journey(), nil)
	assert.Equal(t, synthetic.Cancelled, queued.Run.Outcome)
	assert.Zero(t, queued.Run.StartedUS)
	assert.Equal(t, int32(1), calls.Load())
	close(release)
	require.Equal(t, synthetic.Success, (<-done).Run.Outcome)
	qualified := candidate.Execute(context.Background(), journey(), nil)
	require.Equal(t, synthetic.Success, qualified.Run.Outcome)
	require.NotNil(t, qualified.Run.Runtime)
	assert.Equal(t, "candidate-b", qualified.Run.Runtime.Version)
	monitored := e.Execute(context.Background(), journey(), nil)
	require.Equal(t, synthetic.Success, monitored.Run.Outcome)
	require.NotNil(t, monitored.Run.Runtime)
	assert.Equal(t, "release-a", monitored.Run.Runtime.Version)
	require.NotEmpty(t, history.runs)
	for _, run := range history.runs {
		require.NotNil(t, run.Runtime)
		assert.Equal(t, "release-a", run.Runtime.Version, "candidate work must not enter monitoring history")
	}
}

func TestQualificationViewReportsUnverifiedDrainToMonitoringOwner(t *testing.T) {
	e, _, artifacts := testEngine(t)
	var calls atomic.Int32
	e.start = starter(t, func(context.Context, []string, []byte, *os.File, *os.File) ndexec.TreeResult {
		calls.Add(1)
		return ndexec.TreeResult{Drained: false, Err: errors.New("qualification lost drainage proof")}
	})
	candidate := e.qualificationView(e.source, artifacts)
	result := candidate.Execute(context.Background(), journey(), nil)
	assert.False(t, result.Drained)
	select {
	case err := <-e.Fatal():
		assert.ErrorIs(t, err, ndexec.ErrTreeNotDrained)
	default:
		t.Fatal("candidate failure did not reach the process fatal owner")
	}
	assert.False(t, e.Execute(context.Background(), journey(), nil).Drained)
	assert.Equal(t, int32(1), calls.Load(), "poisoned monitoring cannot start another child")
}

func TestQualificationUsesBothModesWithoutPublishingMonitorHistory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("requires an unprivileged execution identity")
	}
	e, history, _ := testEngine(t)
	paths := e.source.(fixtureRuntime).Paths
	paths.Identity.Version = "candidate-b"
	paths.Identity.SHA256 = strings.Repeat("b", 64)
	var kinds []synthetic.Kind
	e.start = starter(t, func(ctx context.Context, args []string, raw []byte, out, stderr *os.File) ndexec.TreeResult {
		if slices.Contains(args, "--check") {
			_, err := fmt.Fprintf(out, `{"version":"candidate-b","contract":1,"uid":%d,"gid":%d}`, os.Geteuid(), os.Getegid())
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		var request wireRequest
		if err := json.Unmarshal(raw, &request); err != nil {
			return ndexec.TreeResult{Drained: true, Err: err}
		}
		kinds = append(kinds, request.Kind)
		assert.False(t, request.Capture)
		result := success()
		if request.Kind == synthetic.Lighthouse {
			query, err := http.NewRequestWithContext(ctx, http.MethodGet, request.URL, nil)
			if err != nil {
				return ndexec.TreeResult{Drained: true, Err: err}
			}
			response, err := http.DefaultClient.Do(query)
			if err != nil {
				return ndexec.TreeResult{Drained: true, Err: err}
			}
			body, err := io.ReadAll(response.Body)
			_ = response.Body.Close()
			if err != nil {
				return ndexec.TreeResult{Drained: true, Err: err}
			}
			assert.Contains(t, string(body), "DEM runtime qualification")
			score := 95.0
			result.Tests = nil
			result.Metrics = &synthetic.LabMetrics{Performance: &score}
		}
		return resultPayload(result)(ctx, args, raw, out, stderr)
	})
	qualified, err := e.Qualify(context.Background(), fixtureRuntime{Paths: paths})
	require.NoError(t, err)
	assert.True(t, qualified.ExecutionContract)
	assert.True(t, qualified.SandboxedLocalJourney)
	assert.True(t, qualified.SandboxedLocalLighthouse)
	assert.True(t, qualified.ProcessTreeDrained)
	assert.Equal(t, []synthetic.Kind{synthetic.Journey, synthetic.Lighthouse}, kinds)
	assert.Equal(t, paths.Identity, *qualified.Runtime)
	assert.Empty(t, history.runs)
	assert.Equal(t, "release-a", e.source.(fixtureRuntime).Paths.Identity.Version)
}
