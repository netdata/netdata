// SPDX-License-Identifier: GPL-3.0-or-later

package cachestat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

type fakeRuntime struct {
	value   snapshot
	err     error
	closed  int
	sampled chan struct{}
}

func (r *fakeRuntime) Snapshot() (snapshot, error) {
	if r.sampled != nil {
		select {
		case r.sampled <- struct{}{}:
		default:
		}
	}
	return r.value, r.err
}
func (r *fakeRuntime) Close() { r.closed++ }

func testCollector(rt *fakeRuntime) (*Collector, *int) {
	c := New()
	opened := new(int)
	c.probe = func(ctx context.Context) (string, error) { return "__folio_mark_dirty", ctx.Err() }
	c.open = func(_ context.Context, target string) (nativeRuntime, error) {
		if target != "__folio_mark_dirty" {
			return nil, errors.New("wrong target")
		}
		*opened++
		return rt, nil
	}
	return c, opened
}

func TestLifecycle(t *testing.T) {
	ctx := context.Background()
	rt := &fakeRuntime{value: snapshot{Accessed: 101, BufferDirty: 23, Added: 17, AccountDirtied: 5}}
	c, opened := testCollector(rt)
	require.NoError(t, c.Init(ctx))
	require.NoError(t, c.Check(ctx))
	require.Zero(t, *opened, "preflight must not attach probes")
	c.Cleanup(ctx)
	require.Zero(t, rt.closed, "test cleanup must not touch a running backend")

	got, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, map[string]float64{
		"cachestat.mark_page_accessed_total":    101,
		"cachestat.mark_buffer_dirty_total":     23,
		"cachestat.add_to_page_cache_lru_total": 17,
		"cachestat.account_page_dirtied_total":  5,
	}, got)
	collecttest.AssertChartTemplateSchema(t, c.ChartTemplateYAML())
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{"ebpf_poc.page_cache_events": {"accessed", "buffer_dirty", "added", "account_dirtied"}},
	})
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, 1, *opened, "one handle persists between collections")
	c.Cleanup(ctx)
	c.Cleanup(ctx)
	assert.Equal(t, 1, rt.closed)
}

func TestFailedSnapshotAbortsCycle(t *testing.T) {
	rt := &fakeRuntime{value: snapshot{Accessed: 9}}
	c, _ := testCollector(rt)
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	rt.err = errors.New("map unavailable")
	rt.value.Accessed = 99
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "map unavailable")
	got := map[string]float64{}
	c.MetricStore().Read().ForEachSeries(func(name string, _ metrix.LabelView, v float64) { got[name] = v })
	assert.Empty(t, got, "aborted snapshot cycles expose a measurement gap")
	rt.err = nil
	got, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, float64(99), got["cachestat.mark_page_accessed_total"])
}

func TestFailedOpenCanRetry(t *testing.T) {
	rt := &fakeRuntime{}
	c, opened := testCollector(rt)
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	open := c.open
	c.open = func(context.Context, string) (nativeRuntime, error) { return nil, errors.New("load failed") }
	_, err := collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "load failed")
	c.open = open
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, 1, *opened)
	c.Cleanup(context.Background())
	assert.Equal(t, 1, rt.closed)
}

func TestCancelledCollectionDoesNotOpen(t *testing.T) {
	c, opened := testCollector(&fakeRuntime{})
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	assert.Zero(t, *opened)
}

func TestConfig(t *testing.T) {
	for name, decode := range map[string]func([]byte, any) error{"json": json.Unmarshal, "yaml": yaml.Unmarshal} {
		t.Run(name, func(t *testing.T) {
			c := New()
			require.NoError(t, decode([]byte(`{"update_every": 2}`), &c.Config))
			require.NoError(t, c.Init(context.Background()))
			assert.Equal(t, Config{UpdateEvery: 2}, c.Configuration())
		})
	}
	c := New()
	c.UpdateEvery = -1
	require.ErrorContains(t, c.Init(context.Background()), "update_every")
	creator, ok := collectorapi.DefaultRegistry.Lookup("cachestat")
	require.True(t, ok)
	assert.Equal(t, collectorapi.InstancePolicySingle, creator.InstancePolicy)
}

func TestTargets(t *testing.T) {
	base := "0 T mark_page_accessed\n0 T mark_buffer_dirty\n0 T add_to_page_cache_lru\n"
	for name, tc := range map[string]struct {
		text, want string
		fails      bool
	}{
		"modern kernel":                       {text: base + "0 T __folio_mark_dirty\n", want: "__folio_mark_dirty"},
		"candidate preference":                {text: base + "0 T __folio_mark_dirty\n0 t account_page_dirtied\n", want: "account_page_dirtied"},
		"data symbols are not attach targets": {text: base + "0 D account_page_dirtied\n", fails: true},
		"missing core target":                 {text: "0 T account_page_dirtied\n", fails: true},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := resolveTargets(context.Background(), strings.NewReader(tc.text))
			if tc.fails {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestRealJobV2Output(t *testing.T) {
	rt := &fakeRuntime{value: snapshot{Accessed: 101, BufferDirty: 23, Added: 17, AccountDirtied: 5}, sampled: make(chan struct{}, 1)}
	c, opened := testCollector(rt)
	var out bytes.Buffer
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName: "ebpf-poc", Name: "cachestat", ModuleName: "cachestat", FullName: "cachestat",
		Module: c, Out: &out, UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	require.Zero(t, *opened)
	done, ready := make(chan struct{}), make(chan struct{})
	go func() { job.StartManaged(ready); close(done) }()
	<-ready
	t.Cleanup(func() { job.Stop(); <-done; job.Cleanup() })
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ticker.C:
			job.Tick(0)
		case <-deadline.C:
			t.Fatal("job did not collect")
		case <-rt.sampled:
			job.Stop()
			<-done
			wire := out.String()
			assert.Contains(t, wire, "ebpf_poc.page_cache_events")
			assert.Contains(t, wire, "incremental")
			assert.Contains(t, wire, "SET 'accessed' = 101")
			assert.Contains(t, wire, "SET 'account_dirtied' = 5")
			return
		}
	}
}
