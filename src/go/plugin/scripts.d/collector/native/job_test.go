// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJob_CriticalStartupAndRecovery(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "[[ $1 == collect ]]\ncat \"$(dirname \"$0\")/response.json\"\n")
	file := filepath.Join(dir, "response.json")
	require.NoError(t, os.WriteFile(file, []byte(snapshotJSON("critical")), 0644))
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'critical' = 1") })
	first := out.String()
	assert.Contains(t, first, "'fixture.depth'")
	assert.Contains(t, first, "'native_script.check_state'")
	assert.Contains(t, first, "DIMENSION 'processed' 'processed' 'incremental'")
	assert.Contains(t, first, "CLABEL 'region' 'east'")
	depthID := chartID(first, "fixture.depth")
	require.NotEmpty(t, depthID)
	// Atomic file replacement avoids intentionally torn fixtures between ticks.
	next := strings.Replace(snapshotJSON("ok"), `"east"`, `"west"`, 1)
	require.NoError(t, os.WriteFile(file+".next", []byte(next), 0644))
	require.NoError(t, os.Rename(file+".next", file))
	out.Reset()
	tickUntil(t, job, func() bool { return strings.Contains(out.String(), "SET 'ok' = 1") })
	second := out.String()
	assert.Contains(t, second, "SET 'critical' = 0")
	assert.Contains(t, second, "CLABEL 'region' 'west'")
	assert.Contains(t, second, depthID)
	if changedID := chartID(second, "fixture.depth"); changedID != "" {
		assert.Equal(t, depthID, changedID, "promoted label changes keep chart identity")
	}
}

func TestJob_StopCancelsCommand(t *testing.T) {
	setupRunner(t)
	c, dir := fixtureCollector(t, "printf started > \"$(dirname \"$0\")/started\"\nsleep 30\n")
	out := &wireOutput{}
	job, _ := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return fileExists(filepath.Join(dir, "started")) })
	start := time.Now()
	job.Stop()
	assert.Less(t, time.Since(start), 2*time.Second)
	assert.NotContains(t, out.String(), "'native_script.check_state'")
}

func TestJob_PersistentFailureAndReplacement(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
dir=$(dirname "$0")
printf '%s' "$$" >> "$dir/launches"
nd_ready
while nd_next; do
    if [[ -f $dir/fail ]]; then exit 7; fi
    nd_begin
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" 17 queue mail
    nd_check backlog "Queue Backlog" queue
    nd_check_sample "$ND_FAMILY" critical queue mail
    nd_end
done
`)
	out := &wireOutput{}
	job, run := startTestJob(t, c, out)
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
	for i := range 5 {
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
    nd_metric depth gauge jobs
    nd_sample "$ND_FAMILY" 17 queue mail
    nd_check backlog "Queue Backlog" queue
    nd_check_sample "$ND_FAMILY" critical queue mail
    nd_end
done
`)
	nextOut := &wireOutput{}
	nextJob, _ := startTestJob(t, replacement, nextOut)
	tickUntil(t, nextJob, func() bool { return strings.Contains(nextOut.String(), "SET 'critical' = 1") })
	assert.Equal(t, firstID, chartID(nextOut.String(), "native_script.check_state"))
}

func TestJob_PersistentStopDuringCollection(t *testing.T) {
	setupRunner(t)
	c, dir := persistentCollector(t, bashHelper(t)+`
nd_ready
nd_next
printf started > "$(dirname "$0")/started"
sleep 30
`)
	out := &wireOutput{}
	job, run := startTestJob(t, c, out)
	tickUntil(t, job, func() bool { return fileExists(filepath.Join(dir, "started")) })
	job.Stop()
	assert.Nil(t, run.Failure(), "requested stop must not become terminal failure")
	assert.NotContains(t, out.String(), "'native_script.check_state'")
}
