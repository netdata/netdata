//go:build cgo

// SPDX-License-Identifier: GPL-3.0-or-later

package processes

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/apps/collector/processes/processesfunc"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This goes through New -> Init -> real procfs adapter -> grouping -> C finalize
// -> metric store and Function, without injecting already-computed process rows.
func TestNativeCollectionPipeline(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(root, "12"), 0700))
	write := func(name, body string) {
		t.Helper()
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte(body), 0600))
	}
	write("stat", "cpu 100 0 20 1000 0 0 0 0 0 0\ncpu0 100 0 20 1000 0 0 0 0 0 0\n")
	write("uptime", "1000.00 900.00\n")
	fields := make([]string, 53)
	for i := 4; i <= 52; i++ {
		fields[i] = "0"
	}
	fields[4] = "1"
	fields[14] = "10"
	fields[15] = "5"
	fields[20] = "2"
	fields[22] = "100"
	write("12/stat", "12 (worker) S "+strings.Join(fields[4:], " ")+"\n")
	write("12/status", "Name:\tworker\nUid:\t7 7 7 7\nGid:\t8 8 8 8\nVmSize:\t32 kB\nVmRSS:\t12 kB\nVmSwap:\t0 kB\nRssFile:\t4 kB\nRssShmem:\t2 kB\nvoluntary_ctxt_switches:\t10\nnonvoluntary_ctxt_switches:\t20\n")
	write("12/io", "rchar: 100\nwchar: 200\nsyscr: 2\nsyscw: 3\nread_bytes: 50\nwrite_bytes: 60\n")
	write("12/cmdline", "worker\x00")
	c := New()
	c.ProcPath = root
	c.CollectFDs = false
	c.CollectPSS = false
	require.NoError(t, c.Init(context.Background()))
	t.Cleanup(func() { c.Cleanup(context.Background()) })
	require.NoError(t, c.Check(context.Background()))
	metrics, err := collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	for kind, name := range map[string]string{"application": "worker", "user": "7", "group": "8"} {
		key := func(metric string) string {
			return fmt.Sprintf(`%s{group_id="%x",kind=%s,name=%s}`, metric, name, strconv.Quote(kind), strconv.Quote(name))
		}
		require.Contains(t, metrics, key("resident_memory_bytes"))
		assert.Equal(t, float64(12*1024), metrics[key("resident_memory_bytes")])
		assert.Equal(t, 2.0, metrics[key("threads")])
		assert.Equal(t, 1.0, metrics[key("processes")])
		assert.NotContains(t, metrics, key("cpu_user_percent"), "first sample has no rate baseline")
	}
	metrics, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	require.Contains(t, metrics, `cpu_user_percent{group_id="776f726b6572",kind="application",name="worker"}`)
	assert.Equal(t, 0.0, metrics[`cpu_user_percent{group_id="776f726b6572",kind="application",name="worker"}`])
	reply := c.function.Handle(context.Background(), processesfunc.MethodID, funcapi.ResolvedParams{})
	require.Equal(t, 200, reply.Status)
	rows, ok := reply.Data.([][]any)
	require.True(t, ok)
	require.Len(t, rows, 1)
	assert.Equal(t, int32(12), rows[0][1])
	assert.Equal(t, "worker", rows[0][4])

	// Procfs argv contains NUL separators, not shell quotes. Spaces inside a
	// script argument must survive the native boundary and keep its group name.
	write("12/stat", "12 (python3) S "+strings.Join(fields[4:], " ")+"\n")
	write("12/cmdline", "python3\x00/opt/my service.py\x00--flag\x00")
	_, err = collecttest.CollectScalarSeries(c)
	require.NoError(t, err)
	assert.Equal(t, "my service", c.CurrentSnapshot().Processes[0].Application)
}
