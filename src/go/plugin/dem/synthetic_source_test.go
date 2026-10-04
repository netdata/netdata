// SPDX-License-Identifier: GPL-3.0-or-later
package dem_test

import (
	"context"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/syntheticfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRegistryPublishesSyntheticHistoryWithoutActiveJobs(t *testing.T) {
	ctx := context.Background()
	history, err := store.Open(ctx, "")
	require.NoError(t, err)
	defer history.Close()
	captures, err := artifacts.Open("")
	require.NoError(t, err)
	defer captures.Close()
	run := synthetic.Run{ID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", JobID: "journey:retired", Kind: synthetic.Journey, Name: "retired", StartedUS: time.Now().UnixMicro(), Outcome: synthetic.Failed, CaptureState: "disabled", Events: []synthetic.Event{{Kind: "error", Message: "assertion failed"}}}
	run.CompletedUS = run.StartedUS + 1
	_, err = history.AppendSyntheticRun(ctx, "complete", run)
	require.NoError(t, err)
	registry, _ := dem.NewRegistry(dem.Dependencies{History: history, Artifacts: captures}, dem.DefaultConfig())
	require.Contains(t, registry, "journey")
	require.Contains(t, registry, "lighthouse")
	require.Len(t, registry["journey"].AgentFunctions(), 4)
	handler := registry["journey"].MethodHandler(nil).(*syntheticfunc.Handler)
	checks := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "synthetics-checks", Permissions: "0x7b"})
	require.NotNil(t, checks.RawResponse)
	assert.Empty(t, checks.RawResponse["data"])
	detail := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "synthetics-run", Permissions: "0x1b", Args: []string{"job_id:journey:retired", "run_id:" + run.ID}})
	require.NotNil(t, detail.RawResponse, "%v", detail)
	retained := detail.RawResponse["run"].(map[string]any)
	assert.Equal(t, run.ID, retained["id"])
	assert.Equal(t, run.Outcome, retained["outcome"])
	assert.Nil(t, retained["metrics"])
	assert.Len(t, detail.RawResponse["data"], 1)
}
