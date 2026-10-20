// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"bytes"
	"context"
	"errors"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec"
)

type nvidiaSmiBinary interface {
	// start prepares the first query; Check calls it before collecting.
	start(ctx context.Context) error
	queryGPUInfo() ([]byte, error)
	stop() error
}

func newNvidiaSmiBinary(path string, cfg Config, log *logger.Logger) (nvidiaSmiBinary, error) {
	if !cfg.LoopMode {
		if runtime.GOOS == "windows" {
			return &nvidiaSmiDirectExec{
				Logger:  log,
				binPath: path,
				timeout: cfg.Timeout.Duration(),
			}, nil
		}
		return &nvidiaSmiExec{
			Logger:  log,
			binPath: path,
			timeout: cfg.Timeout.Duration(),
		}, nil
	}

	return newNvidiaSmiLoopExec(path, cfg, log)
}

// nvidiaSmiExec executes nvidia-smi via nd-run (Linux/BSD)
type nvidiaSmiExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *nvidiaSmiExec) start(context.Context) error { return nil }

func (e *nvidiaSmiExec) queryGPUInfo() ([]byte, error) {
	return ndexec.RunUnprivileged(e.Logger, e.timeout, e.binPath, "-q", "-x")
}

func (e *nvidiaSmiExec) stop() error { return nil }

// nvidiaSmiDirectExec executes nvidia-smi directly (Windows)
type nvidiaSmiDirectExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *nvidiaSmiDirectExec) start(context.Context) error { return nil }

func (e *nvidiaSmiDirectExec) queryGPUInfo() ([]byte, error) {
	return ndexec.RunDirect(e.Logger, e.timeout, e.binPath, "-q", "-x")
}

func (e *nvidiaSmiDirectExec) stop() error { return nil }

// nvidiaSmiLoopExec keeps nvidia-smi running in loop mode (Linux/BSD) and serves its latest fresh sample.
type nvidiaSmiLoopExec struct {
	source       *streamexec.Source[[]byte]
	startTimeout time.Duration
	stopSource   func()
}

// loopTiming returns the nvidia-smi loop interval in seconds, the data collection interval capped at 5, and how long
// a sample stays current. A query may take up to timeout, so a sample stays current, and the process alive, for one
// loop interval plus timeout.
func loopTiming(updateEvery int, timeout time.Duration) (interval int, freshFor time.Duration) {
	interval = min(updateEvery, 5)
	return interval, time.Duration(interval)*time.Second + timeout
}

func newNvidiaSmiLoopExec(path string, cfg Config, log *logger.Logger) (*nvidiaSmiLoopExec, error) {
	timeout := cfg.Timeout.Duration()
	interval, freshFor := loopTiming(cfg.UpdateEvery, timeout)
	source, err := streamexec.New(streamexec.Config[[]byte]{
		Name: "nvidia-smi",
		Start: func(ctx context.Context, stdout *os.File) (*ndexec.Process, error) {
			log.Debugf("executing '%s -q -x -l %d'", path, interval)
			return ndexec.StartUnprivilegedProcess(ctx, ndexec.ProcessOptions{
				Stdout: stdout,
			},
				path, "-q", "-x", "-l", strconv.Itoa(interval))
		},
		NewDecoder: func() streamexec.Decoder[[]byte] { return &logDecoder{} },
		Timing: streamexec.Timing{
			MaxSampleAge:    freshFor,
			StallTimeout:    freshFor,
			RestartDelayMin: time.Second,
			RestartDelayMax: 30 * time.Second,
		},
		Logger: log,
	})
	if err != nil {
		return nil, err
	}
	return &nvidiaSmiLoopExec{
		source:       source,
		startTimeout: timeout,
	}, nil
}

// start runs nvidia-smi and waits up to the configured timeout for its first sample.
func (e *nvidiaSmiLoopExec) start(ctx context.Context) error {
	if e.stopSource != nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.startTimeout)
	defer cancel()
	stop, err := e.source.Background(ctx)
	if err != nil {
		return err
	}
	e.stopSource = stop
	return nil
}

func (e *nvidiaSmiLoopExec) queryGPUInfo() ([]byte, error) {
	if sample, ok := e.source.Latest(); ok {
		return sample, nil
	}
	return nil, errors.New("no fresh nvidia-smi sample")
}

func (e *nvidiaSmiLoopExec) stop() error {
	if e.stopSource != nil {
		e.stopSource()
		e.stopSource = nil
	}
	return nil
}

// logDecoder assembles each <nvidia_smi_log> document of the loop output into one sample; lines outside a document
// are skipped.
type logDecoder struct {
	buf    bytes.Buffer
	inside bool
}

func (d *logDecoder) Decode(line []byte) ([]byte, bool) {
	switch {
	case string(line) == "<nvidia_smi_log>":
		d.inside = true
		d.buf.Reset()
	case !d.inside:
		return nil, false
	}
	d.buf.Write(line)
	if string(line) != "</nvidia_smi_log>" {
		d.buf.WriteByte('\n')
		return nil, false
	}
	d.inside = false
	return bytes.Clone(d.buf.Bytes()), true
}
