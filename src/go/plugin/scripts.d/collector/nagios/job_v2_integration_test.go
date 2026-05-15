// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/collector/nagios/internal/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNagiosCollectorJobV2(t *testing.T) {
	truePluginPath := writeTestPluginFile(t, "true")

	type jobCaseState struct {
		job         *jobruntime.JobV2
		out         *lockedBuffer
		startedFile string
	}

	tests := map[string]struct {
		setup func(*testing.T) jobCaseState
		run   func(*testing.T, jobCaseState)
	}{
		"emits perfdata-derived metrics on tick": {
			setup: func(t *testing.T) jobCaseState {
				t.Helper()
				now := time.Date(2026, 3, 21, 12, 0, 0, 0, time.UTC)
				runner := &fakeRunner{
					results: []fakeRun{
						{
							result: checkRunResult{
								ServiceState: "OK",
								JobState:     "OK",
								Parsed: output.ParsedOutput{
									Perfdata: []output.PerfDatum{
										{Label: "requests", Unit: "c", Value: 7},
									},
								},
							},
						},
					},
				}
				coll := newTestCollector()
				coll.runner = runner
				coll.now = func() time.Time { return now }
				coll.Config.JobConfig = JobConfig{
					Name:          "jobv2",
					Plugin:        truePluginPath,
					CheckInterval: confDuration(5 * time.Minute),
					RetryInterval: confDuration(1 * time.Minute),
				}
				coll.Config.UpdateEvery = 1

				out := &lockedBuffer{}
				job := newTestJobV2(t, "jobv2", coll, out)
				return jobCaseState{job: job, out: out}
			},
			run: func(t *testing.T, state jobCaseState) {
				t.Helper()
				state.job.Tick(1)
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if state.out.Len() > 0 {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}

				wire := state.out.String()
				assert.Contains(t, wire, "CHART '")
				assert.Contains(t, wire, "BEGIN '")
				assert.Contains(t, wire, "requests")
			},
		},
		"stop cancels in-flight script": {
			setup: func(t *testing.T) jobCaseState {
				t.Helper()
				dir := t.TempDir()
				startedFile := filepath.Join(dir, "started")
				scriptPath := filepath.Join(dir, "check_slow.sh")
				writeExecutable(t, scriptPath, "#!/bin/sh\nset -eu\nstarted_file=\"$1\"\necho started > \"$started_file\"\ntrap 'exit 0' TERM INT\nsleep 30\n")

				coll := newTestCollector()
				coll.Config.JobConfig = JobConfig{
					Name:          "cancel_job",
					Plugin:        scriptPath,
					Args:          []string{startedFile},
					Timeout:       confDuration(30 * time.Second),
					CheckInterval: confDuration(5 * time.Minute),
					RetryInterval: confDuration(1 * time.Minute),
				}
				coll.Config.UpdateEvery = 1

				out := &lockedBuffer{}
				job := newTestJobV2(t, "cancel_job", coll, out)
				return jobCaseState{job: job, out: out, startedFile: startedFile}
			},
			run: func(t *testing.T, state jobCaseState) {
				t.Helper()
				state.job.Tick(1)
				deadline := time.Now().Add(2 * time.Second)
				for time.Now().Before(deadline) {
					if _, err := os.Stat(state.startedFile); err == nil {
						break
					}
					time.Sleep(10 * time.Millisecond)
				}
				_, err := os.Stat(state.startedFile)
				require.NoError(t, err, "timed out waiting for in-flight script start")
				stopStarted := time.Now()
				state.job.Stop()
				assert.LessOrEqual(t, time.Since(stopStarted), 3*time.Second)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			state := tc.setup(t)

			startDone := make(chan struct{})
			go func() {
				state.job.Start()
				close(startDone)
			}()

			waitForJobRunning(t, state.job)

			if name != "stop cancels in-flight script" {
				defer stopAndWaitForJob(t, state.job, startDone)
			}

			tc.run(t, state)

			if name == "stop cancels in-flight script" {
				select {
				case <-startDone:
				case <-time.After(2 * time.Second):
					require.FailNow(t, "timeout waiting for job start loop to exit")
				}
			}
		})
	}
}

func newTestJobV2(t *testing.T, name string, coll *Collector, out io.Writer) *jobruntime.JobV2 {
	t.Helper()
	job := jobruntime.NewJobV2(jobruntime.JobV2Config{
		PluginName:  "scripts.d",
		Name:        name,
		ModuleName:  "nagios",
		FullName:    "scripts_d_" + name,
		Module:      coll,
		Out:         out,
		UpdateEvery: 1,
	})
	require.NoError(t, job.AutoDetection())
	return job
}

func waitForJobRunning(t *testing.T, job *jobruntime.JobV2) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && !job.IsRunning() {
		time.Sleep(10 * time.Millisecond)
	}
	require.True(t, job.IsRunning(), "job did not enter running state")
}

func stopAndWaitForJob(t *testing.T, job *jobruntime.JobV2, startDone <-chan struct{}) {
	t.Helper()
	job.Stop()
	select {
	case <-startDone:
	case <-time.After(2 * time.Second):
		require.FailNow(t, "timeout waiting for job start loop to exit")
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o755), "write executable %s", path)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Len()
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
