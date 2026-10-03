//go:build cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

// BenchmarkPipelineWarmFDs200x20 measures the actual native procfs adapter,
// grouping, metric writing/commit, chart planning/emission/commit and runtime
// metric flush. The static fixture has 200 processes sharing 20 file targets,
// one application, one UID and one GID, with FD and PSS collection enabled.
// Twenty warmup cycles populate FD/PSS caches and chart state. Fixture creation
// and warmup are untimed. This is a warm-filesystem workload with zero counter
// deltas, not a process-churn or live-kernel throughput claim. B/op and allocs/op
// include only Go heap allocations: C malloc/realloc/libc allocations are absent.
func BenchmarkPipelineWarmFDs200x20(b *testing.B) {
	ctx := context.Background()
	c := New()
	c.ProcPath = pipelineProcFixture(b)
	require.NoError(b, c.Init(ctx))
	b.Cleanup(func() { c.Cleanup(ctx) })
	require.NoError(b, c.Check(ctx))
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(b, ok)
	controller := managed.CycleController()
	source, err := collectorapi.NewChartTemplateSource(c)
	require.NoError(b, err)
	aggregator := chartengine.NewRuntimeAggregator(metrix.NewRuntimeStore())
	engine, err := chartengine.New(chartengine.WithRuntimeStore(nil), chartengine.WithRuntimeSampleObserver(aggregator.Observe))
	require.NoError(b, err)
	var output bytes.Buffer
	api := netdataapi.New(&output)
	env := chartemit.EmitEnv{TypeID: "bench.appsgo", UpdateEvery: 1}
	cycle := func() {
		controller.BeginCycle()
		if err := c.Collect(ctx); err != nil {
			controller.AbortCycle()
			b.Fatal(err)
		}
		templates, err := source.Capture()
		if err != nil {
			b.Fatal(err)
		}
		if err := controller.CommitCycleSuccess(); err != nil {
			b.Fatal(err)
		}
		attempt, err := engine.PreparePlanWithOptions(c.MetricStore().Read(metrix.ReadRaw(), metrix.ReadFlatten()), chartengine.PlanOptions{TemplateSet: templates})
		if err != nil {
			b.Fatal(err)
		}
		output.Reset()
		if err := chartemit.ApplyPlan(api, attempt.Plan(), env); err != nil {
			b.Fatal(err)
		}
		if err := attempt.Commit(); err != nil {
			b.Fatal(err)
		}
		aggregator.Flush()
	}
	for range 20 {
		cycle()
	}
	require.Len(b, c.CurrentSnapshot().Processes, 200)
	charts := bytes.Count(output.Bytes(), []byte("BEGIN "))
	require.Positive(b, charts)
	// Exercise all three grouping axes and unique-FD collection through the real
	// store, rather than accepting a scanner that silently skipped fixture rows.
	var fdSeries int
	c.MetricStore().Read(metrix.ReadFlatten()).ForEachByName("unique_fds", func(labels metrix.LabelView, value metrix.SampleValue) {
		if kind, _ := labels.Get("fd_type"); kind == "file" {
			require.Equal(b, 20.0, value)
			fdSeries++
		}
	})
	require.Equal(b, 3, fdSeries)
	b.ReportAllocs()
	b.ResetTimer()
	var reads, links uint64
	for i := 0; i < b.N; i++ {
		cycle()
		s := c.CurrentSnapshot()
		reads += s.Stats.FileReads
		links += s.Stats.FDLinksRead
	}
	b.StopTimer()
	b.ReportMetric(float64(charts), "charts/cycle")
	b.ReportMetric(float64(reads)/float64(b.N), "proc-reads/op")
	b.ReportMetric(float64(links)/float64(b.N), "readlink/op")
}

func pipelineProcFixture(b *testing.B) string {
	b.Helper()
	root := b.TempDir()
	write := func(name, data string) {
		b.Helper()
		p := filepath.Join(root, name)
		require.NoError(b, os.MkdirAll(filepath.Dir(p), 0755))
		require.NoError(b, os.WriteFile(p, []byte(data), 0644))
	}
	write("uptime", "1000 0\n")
	write("stat", "cpu 100 0 100 0 0 0 0 0 0 0\ncpu0 0\n")
	fields := make([]string, 50)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0] = "S"
	for field, value := range map[int]string{4: "1", 10: "200", 12: "100", 14: "100", 20: "2", 22: "1", 23: "1048576", 24: "20"} {
		fields[field-3] = value
	}
	statFields := strings.Join(fields, " ")
	for pid := 2; pid <= 201; pid++ {
		base := strconv.Itoa(pid)
		write(base+"/stat", fmt.Sprintf("%d (worker) %s\n", pid, statFields))
		write(base+"/status", "Uid:\t1000 1000 1000 1000\nGid:\t1001 1001 1001 1001\nVmSize: 1024 kB\nVmRSS: 100 kB\nRssFile: 20 kB\nRssShmem: 10 kB\nVmSwap: 5 kB\nvoluntary_ctxt_switches: 10\nnonvoluntary_ctxt_switches: 2\n")
		write(base+"/io", "read_bytes: 102400\nwrite_bytes: 51200\nrchar: 204800\nwchar: 102400\nsyscr: 200\nsyscw: 100\n")
		write(base+"/cmdline", "worker\x00--fixture\x00")
		write(base+"/limits", "Limit                     Soft Limit           Hard Limit           Units\nMax open files            100                  100                  files\n")
		write(base+"/smaps_rollup", "0000-ffff ---p 00000000 00:00 0 [rollup]\nPss: 60 kB\n")
		require.NoError(b, os.MkdirAll(filepath.Join(root, base, "fd"), 0755))
		for fd := 0; fd < 20; fd++ {
			require.NoError(b, os.Symlink(fmt.Sprintf("/shared/file/%d", fd), filepath.Join(root, base, "fd", strconv.Itoa(fd))))
		}
	}
	return root
}
