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
	job, _ := startManagedTestJob(t, c, out)
	return job
}

func startManagedTestJob(t *testing.T, c *Collector, out *jobOutput) (*jobruntime.JobV2, *jobruntime.ManagedRun) {
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
	run := jobruntime.NewManagedRun(context.Background(), nil)
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		job.Stop()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("job did not stop")
		}
		job.Cleanup()
	})
	require.Eventually(t, run.Running, 3*time.Second, 10*time.Millisecond)
	return job, run
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

func TestPersistentJobFailureAndReplacement(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
dir=$(dirname "$0")
printf '%s' "$$" >> "$dir/launches"
nd_ready
while nd_next; do
    if [[ -f $dir/fail ]]; then exit 7; fi
    nd_begin
    nd_metric depth 17 queue mail
    nd_check backlog critical queue mail
    nd_end
done
`)
	out := &jobOutput{}
	job, run := startManagedTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'critical' = 1") })
	firstID := chartID(out.String(), "native_script.check_state")
	require.NotEmpty(t, firstID)
	launches, err := os.ReadFile(filepath.Join(dir, "launches"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "fail"), nil, 0644))
	tickUntil(t, job, func() bool { return run.Failure() != nil })
	require.True(t, run.Failure().AfterReady())
	assert.False(t, run.Failure().Retryable())
	out.Reset()
	for i := 0; i < 5; i++ {
		job.Tick(i + 10)
	}
	job.Stop() // joins process teardown before replacement
	assert.NotContains(t, out.String(), "SET 'ok' = 1")
	assert.NotContains(t, out.String(), "SET 'critical' = 1")
	after, err := os.ReadFile(filepath.Join(dir, "launches"))
	require.NoError(t, err)
	assert.Equal(t, launches, after, "failure must not respawn the process")

	// A separately initialized job is an explicit replacement, with identical
	// local declarations and chart identity but a new operational process.
	replacement, _ := persistentCollector(t, bashHelper(t)+`
nd_ready
while nd_next; do
    nd_begin
    nd_metric depth 17 queue mail
    nd_check backlog critical queue mail
    nd_end
done
`)
	nextOut := &jobOutput{}
	nextJob := startTestJob(t, replacement, nextOut)
	tickUntil(t, nextJob, func() bool { return strings.Contains(nextOut.String(), "SET 'critical' = 1") })
	assert.Equal(t, firstID, chartID(nextOut.String(), "native_script.check_state"))
}

func TestPersistentJobStopDuringCollection(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
printf started > "$(dirname "$0")/started"
sleep 30
`)
	out := &jobOutput{}
	job, run := startManagedTestJob(t, c, out)
	tickUntil(t, job, func() bool { _, err := os.Stat(filepath.Join(dir, "started")); return err == nil })
	job.Stop()
	assert.Nil(t, run.Failure(), "requested stop must not become terminal failure")
	assert.NotContains(t, out.String(), "'native_script.check_state'")
}
