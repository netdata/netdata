// SPDX-License-Identifier: GPL-3.0-or-later

package postgres

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type relationEmissionCollector struct {
	*Collector
	step    int
	collect func(int)
}

func (*relationEmissionCollector) Check(context.Context) error { return nil }
func (c *relationEmissionCollector) Collect(context.Context) map[string]int64 {
	c.resetMetrics()
	if c.collect != nil {
		c.collect(c.step)
	} else if c.step == 2 {
		for key, m := range c.mx.tables {
			c.removeTableCharts(m)
			delete(c.mx.tables, key)
		}
	} else {
		m := c.getTableMetrics("t", "db", "public")
		m.updated = true
		m.nTupIns = []int64{100, 110, 0, 10000, 10010}[c.step]
	}
	c.step++
	mx := make(map[string]int64)
	c.collectMetrics(mx)
	return mx
}

type emissionFrames chan string

func (w emissionFrames) Write(p []byte) (int, error) { w <- string(p); return len(p), nil }

func relationJobFrames(t *testing.T, c *relationEmissionCollector, count int) []string {
	t.Helper()
	frames := make(emissionFrames, 10)
	j := jobruntime.NewJob(jobruntime.JobConfig{PluginName: "top-proof", ModuleName: "postgres", Name: "selection", FullName: "postgres_selection", Module: c, Out: frames, UpdateEvery: 1})
	require.NoError(t, j.AutoDetectionManaged(context.Background()))
	run := jobruntime.NewManagedRun(context.Background(), nil)
	done := make(chan struct{})
	go func() { j.StartManaged(run); close(done) }()
	<-run.StartupDone()
	t.Cleanup(func() { run.Stop(nil); <-done })
	var output []string
	for i := 0; i < count; i++ {
		if i > 0 {
			time.Sleep(time.Second)
		}
		deadline := time.After(5 * time.Second)
		// Tick deliberately drops events while the runtime has not entered its receive loop.
		// Readiness acknowledges startup, so retry the same clock until the first event is accepted.
		for {
			j.Tick(i + 1)
			select {
			case frame := <-frames:
				output = append(output, frame)
			case <-time.After(25 * time.Millisecond):
				continue
			case <-deadline:
				t.Fatal("no emission")
			}
			break
		}
	}
	run.Stop(nil)
	<-done
	return output
}

func TestRelationSelectionJobEmission(t *testing.T) {
	c := &relationEmissionCollector{Collector: New()}
	output := relationJobFrames(t, c, 5)
	chartID := "postgres_selection.table_t_db_db_schema_public_ops_rows_rate"
	assert.Contains(t, output[0], "CHART '"+chartID+"'")
	assert.Contains(t, output[2], "obsolete")
	assert.NotContains(t, output[2], "BEGIN '"+chartID+"'")
	assert.Contains(t, output[3], "BEGIN '"+chartID+"'\n")
	assert.Contains(t, output[3], "SET 'inserted' = 10000")
	assert.Contains(t, output[3], "BEGIN 'postgres_selection.table_t_db_db_schema_public_ops_rows_hot_ratio'\nSET 'hot' = \nEND")
	var active int
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, "table_") {
			active++
			assert.False(t, chart.IsRemoved())
		}
	}
	assert.Equal(t, len(c.mx.tables["t_db_public"].charts), active)
	// Optionally replay these exact runtime frames through a real Agent in an isolated environment.
	if dir := os.Getenv("NETDATA_POSTGRES_PROTOCOL_DIR"); dir != "" {
		require.NoError(t, os.MkdirAll(dir, 0700))
		for i, frame := range output {
			require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("frame%d", i)), []byte(frame), 0600))
		}
	}
}

func TestTableSourcesJobEmission(t *testing.T) {
	c := &relationEmissionCollector{Collector: New()}
	c.mx.tables["t_db_public"] = newTableMetrics(nil, 1, "t", "db", "public")
	c.collect = func(step int) {
		m := c.mx.tables["t_db_public"]
		m.ioUpdated = true
		m.heapBlksRead.last = int64(100 + 10*step)
		m.heapBlksHit.last = int64(200 + 40*step)
		if step == 1 || step == 3 {
			m.updated = true
			m.nTupIns = int64(100 + 10*step)
		}
	}
	output := relationJobFrames(t, c, 4)
	ioID := "postgres_selection.table_t_db_db_schema_public_io_rate"
	opsID := "postgres_selection.table_t_db_db_schema_public_ops_rows_rate"
	assert.Contains(t, output[0], "CHART '"+ioID+"'")
	assert.NotContains(t, output[0], "CHART '"+opsID+"'")
	assert.Contains(t, output[1], "CHART '"+opsID+"'")
	for step, frame := range output {
		assert.Contains(t, frame, "BEGIN '"+ioID+"'")
		assert.Contains(t, frame, fmt.Sprintf("SET 'disk' = %d", 100+10*step))
		assert.NotContains(t, frame, "obsolete")
		if step > 0 {
			assert.NotContains(t, frame, "CHART '"+ioID+"'")
			assert.Contains(t, frame, "SET 'miss' = 20")
		}
	}
	assert.Contains(t, output[2], "SET 'inserted' = \n")
	assert.NotContains(t, output[2], "SET 'inserted' = 110")
	assert.Contains(t, output[3], "SET 'inserted' = 130")
}
