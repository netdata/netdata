// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jobOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *jobOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *jobOutput) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }
func (b *jobOutput) Reset()         { b.mu.Lock(); defer b.mu.Unlock(); b.buffer.Reset() }

func startTestJob(t *testing.T, c *Collector, out *jobOutput) *jobruntime.JobV2 {
	t.Helper()
	job := jobruntime.NewJobV2(
		jobruntime.JobV2Config{
			PluginName:  "scripts.d",
			Name:        "fixture",
			ModuleName:  "native",
			FullName:    "scripts_d_fixture",
			Module:      c,
			Out:         out,
			UpdateEvery: 1,
		},
	)
	require.NoError(t, job.AutoDetectionManaged(context.Background()))
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(jobruntime.NewManagedRun(context.Background(), nil)) }()
	t.Cleanup(func() {
		job.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("job did not stop")
		}
		job.Cleanup()
	})
	require.Eventually(t, job.IsRunning, 2*time.Second, 10*time.Millisecond)
	return job
}

func tickUntil(t *testing.T, job *jobruntime.JobV2, condition func() bool) {
	t.Helper()
	require.Eventually(t, func() bool { job.Tick(1); return condition() }, 3*time.Second, 100*time.Millisecond)
}

func TestJobCriticalStartupAndRecovery(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "[[ $1 == collect ]]\ncat \"$(dirname \"$0\")/response.json\"\n")
	file := filepath.Join(dir, "response.json")
	require.NoError(t, os.WriteFile(file, []byte(validResponse("critical")), 0644))
	out := &jobOutput{}
	job := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'critical' = 1") })
	first := out.String()
	assert.Contains(t, first, "'fixture.depth'")
	assert.Contains(t, first, "'native_script.check_state'")
	assert.Contains(t, first, "DIMENSION 'processed' 'processed' 'incremental'")
	assert.Contains(t, first, "CLABEL 'region' 'east'")
	depthID := chartID(first, "fixture.depth")
	require.NotEmpty(t, depthID)
	// Atomic file replacement avoids intentionally torn fixtures between ticks.
	next := strings.Replace(validResponse("ok"), `"east"`, `"west"`, 1)
	require.NoError(t, os.WriteFile(file+".next", []byte(next), 0644))
	require.NoError(t, os.Rename(file+".next", file))
	out.Reset()
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'ok' = 1") })
	second := out.String()
	assert.Contains(t, second, "SET 'critical' = 0")
	assert.Contains(t, second, "CLABEL 'region' 'west'")
	assert.Contains(t, second, depthID)
	if changedID := chartID(second, "fixture.depth"); changedID != "" {
		assert.Equal(t, depthID, changedID)
	}
}

func chartID(wire, contextName string) string {
	for _, line := range strings.Split(wire, "\n") {
		if strings.HasPrefix(line, "CHART '") && strings.Contains(line, "'"+contextName+"'") {
			return strings.SplitN(line, "'", 3)[1]
		}
	}
	return ""
}

func TestJobStopCancelsCommand(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "printf started > \"$(dirname \"$0\")/started\"\nsleep 30\n")
	out := &jobOutput{}
	job := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { _, err := os.Stat(filepath.Join(dir, "started")); return err == nil })
	start := time.Now()
	job.Stop()
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.NotContains(t, out.String(), "'native_script.check_state'")
}
