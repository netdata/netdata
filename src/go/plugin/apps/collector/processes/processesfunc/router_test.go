// SPDX-License-Identifier: GPL-3.0-or-later
package processesfunc

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type snapshotDeps struct{ snapshot *model.Snapshot }

func (d *snapshotDeps) CurrentSnapshot() *model.Snapshot { return d.snapshot }
func TestProcessResponse(t *testing.T) {
	p := model.Process{Key: model.Key{PID: 21, StartTime: 300}, PPID: 1, Comm: "python3", Cmdline: "python3 /service.py --password synthetic-secret", Application: "service", UID: 1000, GID: 1001, State: "R", Valid: (1 << model.MetricCount) - 1}
	p.Values[model.CPUUser] = 12
	p.Values[model.CPUSystem] = 3
	p.Values[model.ResidentMemory] = 4096
	p.Valid &^= 1 << model.ProportionalMemory
	s := &model.Snapshot{CollectedAt: time.Date(2026, 9, 25, 1, 2, 3, 0, time.UTC), Processes: []model.Process{p}}
	r := NewRouter(&snapshotDeps{snapshot: s})
	resp := r.Handle(context.Background(), MethodID, nil)
	require.Equal(t, 200, resp.Status)
	require.Len(t, resp.Data, 1)
	rows, ok := resp.Data.([][]any)
	require.True(t, ok)
	assert.Equal(t, "21:300", rows[0][0])
	assert.Equal(t, 15.0, rows[0][8])
	assert.Nil(t, rows[0][10])
	assert.Equal(t, 4096.0, rows[0][9])
	assert.Contains(t, resp.Help, "2026-09-25T01:02:03Z")
	assert.Contains(t, resp.Help, "age")
	data, err := json.Marshal(resp)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "synthetic-secret")
	assert.NotContains(t, string(data), "--password")
	assert.Equal(t, p, s.Processes[0], "Function leaves published data untouched")
	params, err := r.MethodParams(context.Background(), MethodID)
	require.NoError(t, err)
	require.Len(t, params, 1)
	assert.Equal(t, "app:service", params[0].Options[1].ID)
	filtered := r.Handle(context.Background(), MethodID, funcapi.ResolvedParams{"application": {IDs: []string{"app:other"}}})
	assert.Empty(t, filtered.Data)
	filtered = r.Handle(context.Background(), MethodID, funcapi.ResolvedParams{"application": {IDs: []string{"app:service"}}})
	assert.Len(t, filtered.Data, 1)
}
func TestAvailabilityAndDispatch(t *testing.T) {
	r := NewRouter(&snapshotDeps{})
	assert.Equal(t, 503, r.Handle(context.Background(), MethodID, nil).Status)
	assert.Equal(t, 404, r.Handle(context.Background(), "unknown", nil).Status)
	_, err := r.MethodParams(context.Background(), "unknown")
	assert.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	assert.Equal(t, 499, r.Handle(ctx, MethodID, nil).Status)
	r.Cleanup(context.Background())
	assert.Equal(t, FunctionName, Methods(1)[0].FunctionName)
}
