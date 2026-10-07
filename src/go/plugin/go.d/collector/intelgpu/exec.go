// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec"
)

type intelGpuTop interface {
	// start runs intel_gpu_top and waits for its first sample; Check calls it before collecting.
	start(ctx context.Context) error
	queryGPUSummaryJson() ([]byte, error)
	stop()
}

// firstSampleTimeout bounds the wait for the first sample. intel_gpu_top prints a sample at once but completes it only
// when it prints the next one, one sampling interval later, so the interval stays below this timeout.
const firstSampleTimeout = 3 * time.Second

// intelGpuTopExec keeps intel_gpu_top running through ndsudo and serves its latest fresh sample.
type intelGpuTopExec struct {
	source     *streamexec.Source[[]byte]
	stopSource func()
}

func newIntelGpuTopExec(log *logger.Logger, updateEvery int, device string) (*intelGpuTopExec, error) {
	interval := calcInterval(updateEvery)
	command, args := "igt-json", []string{"--interval", strconv.FormatInt(interval.Milliseconds(), 10)}
	if device != "" {
		command, args = "igt-device-json", append(args, "--device", device)
	}
	source, err := streamexec.New(streamexec.Config[[]byte]{
		Name: "intel_gpu_top",
		Start: func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
			return ndexec.StartNDSudoProcess(ctx, ndexec.ProcessOptions{
				Stdout: stdout,
			}, command, args...)
		},
		NewDecoder: func() streamexec.Decoder[[]byte] { return &sampleDecoder{} },
		Timing:     sampleTiming(interval),
		Logger:     log,
	})
	if err != nil {
		return nil, err
	}
	return &intelGpuTopExec{
		source: source,
	}, nil
}

// start runs intel_gpu_top and waits up to firstSampleTimeout for its first sample.
func (e *intelGpuTopExec) start(ctx context.Context) error {
	if e.stopSource != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, firstSampleTimeout)
	defer cancel()
	stop, err := e.source.Background(ctx)
	if err != nil {
		return err
	}
	e.stopSource = stop
	return nil
}

func (e *intelGpuTopExec) queryGPUSummaryJson() ([]byte, error) {
	if sample, ok := e.source.Latest(); ok {
		return sample, nil
	}
	return nil, errors.New("no fresh intel_gpu_top sample")
}

func (e *intelGpuTopExec) stop() {
	if e.stopSource != nil {
		e.stopSource()
		e.stopSource = nil
	}
}

// calcInterval returns the intel_gpu_top sampling interval for the data collection interval. It stays below
// firstSampleTimeout, because a sample is complete only when the next one starts.
func calcInterval(updateEvery int) time.Duration {
	if m := min(updateEvery, int(firstSampleTimeout.Seconds())); m > 1 {
		return time.Duration(m)*time.Second - 500*time.Millisecond
	}
	return 900 * time.Millisecond
}

// sampleTiming keeps a sample current for two sampling intervals and replaces intel_gpu_top after three without one.
// The stall timeout also bounds Cleanup's wait for intel_gpu_top, which go.d.plugin cannot signal, to exit.
func sampleTiming(interval time.Duration) streamexec.Timing {
	return streamexec.Timing{
		MaxSampleAge:    2 * interval,
		StallTimeout:    3 * interval,
		RestartDelayMin: time.Second,
		RestartDelayMax: 30 * time.Second,
	}
}

// sampleDecoder assembles each top-level JSON object of the intel_gpu_top output into one sample, from a "{" line to
// a line starting with "}"; lines outside an object are skipped. intel_gpu_top ends a sample's closing line with ","
// when it starts the next one; the comma is dropped.
type sampleDecoder struct {
	buf bytes.Buffer
}

func (d *sampleDecoder) Decode(line []byte) ([]byte, bool) {
	if len(line) == 0 || d.buf.Len() == 0 && string(line) != "{" {
		return nil, false
	}
	if string(line) == "}," {
		line = line[:1]
	}
	d.buf.Write(line)
	d.buf.WriteByte('\n')
	if line[0] != '}' {
		return nil, false
	}
	sample := bytes.Clone(d.buf.Bytes())
	d.buf.Reset()
	return sample, true
}
