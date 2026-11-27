package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"
)

// TestProcessGlobalChartContract pins the chart identity the C process module
// published (ebpf_process.c ebpf_create_global_charts).  Changing any of these
// splits existing chart instances on upgrade.
func TestProcessGlobalChartContract(t *testing.T) {
	type chartContract struct {
		context    string
		units      string
		order      int
		dimensions []processGlobalDimension
		errorChart bool
	}
	absolute := func(idName ...string) []processGlobalDimension {
		dims := make([]processGlobalDimension, 0, len(idName)/2)
		for i := 0; i < len(idName); i += 2 {
			dims = append(dims, processGlobalDimension{idName[i], idName[i+1], "absolute"})
		}
		return dims
	}
	want := map[string]chartContract{
		"process_thread": {context: "system.process_thread", units: "calls/s", order: 21002, dimensions: absolute("_do_fork", "process", "sys_clone", "thread")},
		"exit":           {context: "system.exit", units: "calls/s", order: 21003, dimensions: absolute("do_exit", "process", "release_task", "task")},
		"process_status": {
			context: "system.process_status", units: "calls", order: 21004,
			dimensions: absolute("process", "process", "zombie", "zombie"),
		},
		"task_error": {context: "system.task_error", units: "calls/s", order: 21005, dimensions: absolute("_do_fork", "process", "sys_clone", "thread"), errorChart: true},
	}

	got := make(map[string]chartContract, len(processGlobalCharts))
	for _, c := range processGlobalCharts {
		got[c.id] = chartContract{c.context, c.units, c.order, c.dimensions, c.errorChart}
	}

	if len(got) != len(want) {
		t.Fatalf("charts = %v, want %v", got, want)
	}
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Fatalf("missing chart %q", id)
		}
		if g.context != w.context || g.units != w.units || g.order != w.order || g.errorChart != w.errorChart ||
			len(g.dimensions) != len(w.dimensions) {
			t.Fatalf("chart %q = %+v, want %+v", id, g, w)
		}
		for i := range w.dimensions {
			if g.dimensions[i] != w.dimensions[i] {
				t.Fatalf("chart %q dimensions = %v, want %v", id, g.dimensions, w.dimensions)
			}
		}
	}
}

func TestProcessGlobalInterval(t *testing.T) {
	first := libbpfloader.ProcessSnapshot{Exits: 10, TaskClose: 8, Process: 20, ProcessErr: 1, Thread: 5, ThreadErr: 2}
	second := libbpfloader.ProcessSnapshot{Exits: 16, TaskClose: 10, Process: 30, ProcessErr: 4, Thread: 9, ThreadErr: 2}

	var state processGlobalState
	// The first interval counts from zero, as the C module's did.
	if got, want := state.interval(first), (processInterval{
		exits: 10, taskClose: 8, process: 20, processErr: 1, thread: 5, threadErr: 2, running: 15, zombie: 2,
	}); got != want {
		t.Fatalf("first interval = %+v, want %+v", got, want)
	}
	if got, want := state.interval(second), (processInterval{
		exits: 6, taskClose: 2, process: 10, processErr: 3, thread: 4, threadErr: 0, running: 6, zombie: 4,
	}); got != want {
		t.Fatalf("interval = %+v, want %+v", got, want)
	}
}

func TestWriteProcessGlobalSendsIntervalDifferences(t *testing.T) {
	in := processInterval{exits: 3, taskClose: 2, process: 11, processErr: 1, thread: 4, threadErr: 5, running: 7, zombie: 1}
	cases := map[string]struct {
		reportErrors bool
		want         string
	}{
		"entry mode hides errors": {
			want: "BEGIN 'system.process_thread' 100\nSET '_do_fork' = 11\nSET 'sys_clone' = 4\nEND\n" +
				"BEGIN 'system.exit' 100\nSET 'do_exit' = 3\nSET 'release_task' = 2\nEND\n" +
				"BEGIN 'system.process_status' 100\nSET 'process' = 7\nSET 'zombie' = 1\nEND\n",
		},
		"return mode adds errors": {
			reportErrors: true,
			want: "BEGIN 'system.process_thread' 100\nSET '_do_fork' = 11\nSET 'sys_clone' = 4\nEND\n" +
				"BEGIN 'system.exit' 100\nSET 'do_exit' = 3\nSET 'release_task' = 2\nEND\n" +
				"BEGIN 'system.process_status' 100\nSET 'process' = 7\nSET 'zombie' = 1\nEND\n" +
				"BEGIN 'system.task_error' 100\nSET '_do_fork' = 1\nSET 'sys_clone' = 5\nEND\n",
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			writeProcessGlobal(netdataapi.New(&buf), in, 100, tc.reportErrors)
			if got := strings.ReplaceAll(buf.String(), "\n\n", "\n"); got != tc.want {
				t.Fatalf("output:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}
