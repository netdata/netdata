// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"context"
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/grouping"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/native"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

//go:embed testdata/config.json
var configJSON []byte

//go:embed testdata/config.yaml
var configYAML []byte

func TestConfiguration(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, New(), configJSON, configYAML)
}

func TestInitValidation(t *testing.T) {
	for name, change := range map[string]func(*Collector){
		"relative procfs": func(c *Collector) { c.ProcPath = "proc" },
		"zero interval":   func(c *Collector) { c.UpdateEvery = 0 },
		"invalid matcher": func(c *Collector) {
			c.Groups = []grouping.Rule{{Name: "web", Match: []grouping.Match{{Comm: "regexp:["}}}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			change(c)
			c.newScanner = func(native.Options) (scanner, error) {
				t.Fatal("invalid configuration reached native scanner")
				return nil, nil
			}
			require.Error(t, c.Init(context.Background()))
			c.Cleanup(context.Background())
			c.Cleanup(context.Background())
		})
	}
}

type fixtureScanner struct {
	scans       int
	fail        bool
	closed      int
	assignments []model.Assignment
}

func (s *fixtureScanner) Scan(context.Context) (model.Snapshot, error) {
	if s.fail {
		return model.Snapshot{}, errors.New("procfs unavailable")
	}
	s.scans++
	p := model.Process{Key: model.Key{PID: 12, StartTime: 50}, PPID: 1, Comm: "worker", UID: 7, GID: 8, State: "R", Valid: (1 << model.MetricCount) - 1, FDValid: true}
	p.Values = [model.MetricCount]float64{20, 10, 0, 5, 1, 0, 4, 2, 1, 0, 8192, 4096, 1024, 0, 3072, 3072, 100, 200, 300, 400, 2, 3, 5, 6, 4, 60, 25, 3}
	return model.Snapshot{Generation: uint64(s.scans), CollectedAt: time.Unix(1000+int64(s.scans), 0), Processes: []model.Process{p}, Stats: model.ScanStats{FileReads: 8, FDLinksRead: 2}}, nil
}
func (s *fixtureScanner) Finalize(g uint64, a []model.Assignment) ([]model.GroupFD, error) {
	if g != uint64(s.scans) {
		return nil, errors.New("wrong generation")
	}
	s.assignments = append([]model.Assignment(nil), a...)
	var out []model.GroupFD
	for _, assignment := range a {
		for _, id := range assignment.Groups {
			out = append(out, model.GroupFD{ID: id, Valid: true, Counts: [model.FDTypeCount]uint64{1, 1}})
		}
	}
	return out, nil
}
func (s *fixtureScanner) Close() { s.closed++ }

func fixtureCollector(t *testing.T) (*Collector, *fixtureScanner) {
	t.Helper()
	c := New()
	c.ProcPath = t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(c.ProcPath, "stat"), []byte("cpu 1 2 3 4\n"), 0600))
	s := &fixtureScanner{}
	c.newScanner = func(opts native.Options) (scanner, error) {
		assert.Equal(t, c.ProcPath, opts.ProcPath)
		return s, nil
	}
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	return c, s
}

func TestCollectAndChartCoverage(t *testing.T) {
	c, s := fixtureCollector(t)
	values, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Len(t, s.assignments, 1)
	assert.Equal(t, model.Key{PID: 12, StartTime: 50}, s.assignments[0].Key)
	assert.NotZero(t, s.assignments[0].Groups[0])
	assert.NotZero(t, s.assignments[0].Groups[1])
	assert.NotZero(t, s.assignments[0].Groups[2])
	// The raw source values are already rates. The V2 path must not derive them again.
	assert.Equal(t, 20.0, values[`cpu_user_percent{group_id="776f726b6572",kind="application",name="worker"}`])
	assert.Equal(t, 4096.0, values[`resident_memory_bytes{group_id="776f726b6572",kind="application",name="worker"}`])
	assert.Equal(t, 1.0, values[`unique_fds{fd_type="file",group_id="776f726b6572",kind="application",name="worker"}`])
	assert.Equal(t, 1.0, values[`process_state_count{state="running"}`])
	assert.Equal(t, 0.0, values[`process_state_count{state="zombie"}`])
	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{RequiredContexts: map[string][]string{
		"appsgo.cpu":        {"user", "system", "guest", "exited children user", "exited children system", "exited children guest"},
		"appsgo.unique_fds": {"file", "socket", "pipe", "inotify", "event", "timer", "signal", "epoll", "other"},
	}})
	require.NotNil(t, c.CurrentSnapshot())
	assert.Equal(t, "worker", c.CurrentSnapshot().Processes[0].Application)
}

func TestFailedScanKeepsLastCompletedSnapshot(t *testing.T) {
	c, s := fixtureCollector(t)
	_, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	previous := c.CurrentSnapshot()
	s.fail = true
	_, err = collecttest.CollectScalarSeries(c)
	require.ErrorContains(t, err, "procfs unavailable")
	assert.Same(t, previous, c.CurrentSnapshot())
	c.Cleanup(context.Background())
	c.Cleanup(context.Background())
	assert.Equal(t, 1, s.closed)
	assert.Nil(t, c.CurrentSnapshot())
	require.Error(t, c.Collect(context.Background()))
}

func TestCanceledCollectDoesNotScan(t *testing.T) {
	c, s := fixtureCollector(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, c.Collect(ctx), context.Canceled)
	assert.Zero(t, s.scans)
}
