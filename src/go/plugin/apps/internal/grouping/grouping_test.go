// SPDX-License-Identifier: GPL-3.0-or-later
package grouping

import (
	"fmt"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/apps/internal/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func process(pid, ppid int32, comm string) model.Process {
	return model.Process{Key: model.Key{PID: pid, StartTime: 100}, PPID: ppid, Comm: comm}
}
func TestRulesAndTreeLifecycle(t *testing.T) {
	e, err := New([]Rule{{Name: "service", Match: []Match{{Comm: "= server", Cmdline: "~ --serve"}, {Comm: "= alternate"}}}, {Name: "later", Match: []Match{{Comm: "= server"}}}})
	require.NoError(t, err)
	root := process(10, 1, "server")
	root.Cmdline = "server --serve"
	child := process(11, 10, "worker")
	manager := process(12, 10, "systemd")
	service := process(13, 12, "separate")
	snapshot := model.Snapshot{Processes: []model.Process{child, service, manager, root, process(14, 99, "orphan"), process(15, 1, "alternate")}}
	_, a, err := e.Aggregate(&snapshot)
	require.NoError(t, err)
	assert.Equal(t, []string{"service", "separate", "systemd", "service", "orphan", "service"}, applications(snapshot))
	assert.Equal(t, a[0].Groups[0], a[3].Groups[0])
	oldID := a[0].Groups[0]
	root.Comm = "new-server"
	root.Cmdline = "new-server"
	root.Key.StartTime = 200
	snapshot = model.Snapshot{Processes: []model.Process{root, child}}
	_, a, err = e.Aggregate(&snapshot)
	require.NoError(t, err)
	assert.Equal(t, []string{"new-server", "worker"}, applications(snapshot), "a reused younger parent cannot adopt an older child")
	assert.NotEqual(t, oldID, a[0].Groups[0])
	root.Key.StartTime = 50
	snapshot = model.Snapshot{Processes: []model.Process{root, child}}
	_, again, err := e.Aggregate(&snapshot)
	require.NoError(t, err)
	assert.Equal(t, a[0].Groups[0], again[0].Groups[0], "live group retains its ID")
	assert.Equal(t, []string{"new-server", "new-server"}, applications(snapshot))
	_, _, err = e.Aggregate(&model.Snapshot{})
	require.NoError(t, err)
	assert.Empty(t, e.ids)
	root.Comm = "server"
	root.Cmdline = "server --serve"
	_, again, err = e.Aggregate(&model.Snapshot{Processes: []model.Process{root}})
	require.NoError(t, err)
	assert.NotEqual(t, oldID, again[0].Groups[0], "retired IDs are not reused")
}
func applications(s model.Snapshot) []string {
	out := make([]string, len(s.Processes))
	for i, p := range s.Processes {
		out[i] = p.Application
	}
	return out
}
func TestRuleValidation(t *testing.T) {
	for _, rules := range [][]Rule{{{Name: " "}}, {{Name: "x"}}, {{Name: "x", Match: []Match{{}}}}, {{Name: "x", Match: []Match{{Comm: "~ ["}}}}, {{Name: "x", Match: []Match{{Cmdline: "bad"}}}}, {{Name: "x", Match: []Match{{Comm: "= x"}}}, {Name: "x", Match: []Match{{Comm: "= y"}}}}} {
		_, err := New(rules)
		assert.Error(t, err)
	}
	e, err := New([]Rule{{Name: "both", Match: []Match{{Comm: "= server", Cmdline: "~ serve"}}}})
	require.NoError(t, err)
	s := model.Snapshot{Processes: []model.Process{process(1, 0, "server")}}
	_, _, err = e.Aggregate(&s)
	require.NoError(t, err)
	assert.Equal(t, "server", s.Processes[0].Application)
}
func TestCyclesAndNames(t *testing.T) {
	e, err := New(nil)
	require.NoError(t, err)
	for _, pids := range [][]int32{{8, 9}, {9, 8}} {
		ps := []model.Process{process(pids[0], 17-pids[0], fmt.Sprint(pids[0])), process(pids[1], 17-pids[1], fmt.Sprint(pids[1]))}
		s := model.Snapshot{Processes: ps}
		_, _, err = e.Aggregate(&s)
		require.NoError(t, err)
		assert.Equal(t, []string{"8", "8"}, applications(s))
	}
	cases := []struct{ comm, cmdline, want string }{{"python3", "python3 '/opt/my service.py' --token secret", "my service"}, {"python3", "python3 -c 'sensitive-code'", "python3"}, {"bash", "", "bash"}, {"node", "node\x00/opt/service.js\x00--secret\x00", "service"}, {"containerd-shim", "/usr/bin/containerd-shim-runc-v2 -id x", "containerd-shim-runc-v2"}}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			p := process(10, 1, tc.comm)
			p.Cmdline = tc.cmdline
			s := model.Snapshot{Processes: []model.Process{p}}
			_, _, err := e.Aggregate(&s)
			require.NoError(t, err)
			assert.Equal(t, tc.want, s.Processes[0].Application)
			assert.Equal(t, tc.comm, s.Processes[0].Comm)
		})
	}
}
func TestAggregatesAndMissing(t *testing.T) {
	e, err := New(nil)
	require.NoError(t, err)
	p := process(2, 1, "app")
	p.Valid = (1 << model.MetricCount) - 1
	p.Values[model.ResidentMemory] = 100
	p.Values[model.Uptime] = 20
	p.Values[model.FDLimitPercent] = 15
	p.Values[model.PSSAge] = 8
	q := p
	q.Key.PID = 3
	q.Values[model.ResidentMemory] = 50
	q.Values[model.Uptime] = 10
	q.Values[model.FDLimitPercent] = 35
	q.Values[model.PSSAge] = 3
	q.Valid &^= 1 << model.ReadBytes
	groups, a, err := e.Aggregate(&model.Snapshot{Processes: []model.Process{p, q}})
	require.NoError(t, err)
	require.Len(t, groups, 3)
	for _, g := range groups {
		assert.EqualValues(t, 2, g.Processes)
		assert.Equal(t, 150.0, g.Values[model.ResidentMemory])
		assert.Equal(t, 10.0, g.Values[model.Uptime])
		assert.Equal(t, 20.0, g.UptimeMax)
		assert.Equal(t, 35.0, g.Values[model.FDLimitPercent])
		assert.Equal(t, 8.0, g.Values[model.PSSAge])
		assert.False(t, g.Has(model.ReadBytes))
		assert.False(t, g.FDValid)
	}
	assert.Equal(t, a[0].Groups, a[1].Groups)
}
func TestNormalization(t *testing.T) {
	tests := []struct {
		name                           string
		global                         float64
		valid                          bool
		own, child, wantOwn, wantChild float64
	}{{"missing global", 0, false, 80, 40, 80, 40}, {"all fits", 150, true, 80, 40, 80, 40}, {"clip children", 100, true, 80, 40, 80, 20}, {"clip running", 40, true, 80, 40, 40, 0}, {"valid idle", 0, true, 80, 40, 0, 0}}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := New(nil)
			require.NoError(t, err)
			p := process(2, 1, "app")
			p.Valid = (1 << model.MetricCount) - 1
			p.Values[model.CPUUser] = tc.own
			p.Values[model.CPUChildrenUser] = tc.child
			p.Values[model.MinorFaults] = 8
			p.Values[model.ChildrenMinorFaults] = 4
			s := model.Snapshot{Processes: []model.Process{p}, SystemCPU: [3]float64{tc.global, 0, 0}, SystemCPUValid: tc.valid, CPUCount: 2}
			groups, _, err := e.Aggregate(&s)
			require.NoError(t, err)
			for _, g := range groups {
				assert.InDelta(t, tc.wantOwn, g.Values[model.CPUUser], 0.001)
				assert.InDelta(t, tc.wantChild, g.Values[model.CPUChildrenUser], 0.001)
				assert.InDelta(t, tc.wantOwn/10, g.Values[model.MinorFaults], 0.001)
				assert.InDelta(t, tc.wantChild/10, g.Values[model.ChildrenMinorFaults], 0.001)
			}
			assert.Equal(t, groups[0].Values[model.CPUUser], s.Processes[0].Values[model.CPUUser])
		})
	}
}
func BenchmarkAggregate(b *testing.B) {
	e, _ := New(nil)
	ps := make([]model.Process, 2000)
	for i := range ps {
		ps[i] = process(int32(i+2), int32(i+1), "service")
		ps[i].Valid = (1 << model.MetricCount) - 1
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		s := model.Snapshot{Processes: ps}
		if _, _, err := e.Aggregate(&s); err != nil {
			b.Fatal(err)
		}
	}
}

func TestKernelAndExplicitManagerRules(t *testing.T) {
	e, err := New([]Rule{{Name: "orchestrator", Match: []Match{{Comm: "= systemd"}}}})
	require.NoError(t, err)
	s := model.Snapshot{Processes: []model.Process{process(2, 0, "kthreadd"), process(3, 2, "kworker"), process(10, 1, "systemd"), process(11, 10, "service")}}
	_, _, err = e.Aggregate(&s)
	require.NoError(t, err)
	assert.Equal(t, []string{"kernel", "kernel", "orchestrator", "service"}, applications(s))
}

func TestRulesUseRenderedNativeArguments(t *testing.T) {
	e, err := New([]Rule{{Name: "matched", Match: []Match{{Cmdline: "= python3 /opt/my service.py --flag"}}}})
	require.NoError(t, err)
	p := process(12, 1, "python3")
	p.Cmdline = "python3\x00/opt/my service.py\x00--flag\x00"
	s := model.Snapshot{Processes: []model.Process{p}}
	_, _, err = e.Aggregate(&s)
	require.NoError(t, err)
	assert.Equal(t, "matched", s.Processes[0].Application)
}
