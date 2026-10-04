// SPDX-License-Identifier: GPL-3.0-or-later
package dem_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/dem"
	"github.com/netdata/netdata/go/plugins/plugin/dem/artifacts"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/syntheticfunc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
	components := dem.New(dem.Dependencies{History: history, Artifacts: captures}, dem.DefaultConfig())
	require.Contains(t, components.Collectors, "journey")
	require.Contains(t, components.Collectors, "lighthouse")
	require.Len(t, components.Functions[1].Functions(), 4)
	handler := components.Functions[1].NewHandler().(*syntheticfunc.Handler)
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

// This executor makes inventory preparation independent of an installed browser.
// An inventory read must never execute a workflow.
type inventoryExecutor struct{}

func (inventoryExecutor) Check(context.Context, synthetic.Kind) error { return nil }
func (inventoryExecutor) Execute(context.Context, synthetic.Request, func(string)) synthetic.Execution {
	panic("inventory must not execute a browser")
}

func TestRegistrySyntheticInventoryRedactsTargetCredentials(t *testing.T) {
	components := dem.New(dem.Dependencies{Executor: inventoryExecutor{}}, dem.DefaultConfig())
	collector := components.Collectors["lighthouse"].CreateV2().(*lighthouse.Collector)
	collector.Name = "home"
	collector.URL = "https://example.org/?token=fixture-token&password=fixture-password&page=home"
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- collector.Run(ctx, func() { close(ready) }) }()
	defer func() { cancel(); require.NoError(t, <-done) }()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("collector did not become ready")
	}
	handler := components.Functions[1].NewHandler().(*syntheticfunc.Handler)
	checks := handler.HandleRaw(ctx, funcapi.RawMethodRequest{Method: "synthetics-checks", Permissions: "0x1b"})
	require.NotNil(t, checks.RawResponse)
	require.Len(t, checks.RawResponse["data"], 1)
	encoded, err := json.Marshal(checks.RawResponse["data"])
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "fixture-token")
	assert.NotContains(t, string(encoded), "fixture-password")
	assert.Contains(t, string(encoded), "[REDACTED]")
	assert.Equal(t, "https://example.org/?token=fixture-token&password=fixture-password&page=home", collector.URL)
}
