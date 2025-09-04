// SPDX-License-Identifier: GPL-3.0-or-later

package nvidia_smi

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/cmd"
)

type nvidiaSmiBinary interface {
	queryGPUInfo() ([]byte, error)
	stop() error
}

func newNvidiaSmiBinary(path string, cfg Config, log *logger.Logger) (nvidiaSmiBinary, error) {
	if !cfg.LoopMode {
		return &nvidiaSmiExec{
			Logger:  log,
			binPath: path,
			timeout: cfg.Timeout.Duration(),
		}, nil
	}

	smi := &nvidiaSmiLoopExec{
		Logger:             log,
		binPath:            path,
		updateEvery:        cfg.UpdateEvery,
		firstSampleTimeout: cfg.Timeout.Duration(),
	}

	if err := smi.run(); err != nil {
		return nil, err
	}

	return smi, nil
}

type nvidiaSmiExec struct {
	*logger.Logger

	binPath string
	timeout time.Duration
}

func (e *nvidiaSmiExec) queryGPUInfo() ([]byte, error) {
	return cmd.RunUnprivileged(e.Logger, e.timeout, e.binPath, "-q", "-x")
}

func (e *nvidiaSmiExec) stop() error { return nil }

type nvidiaSmiLoopExec struct {
	*logger.Logger

	binPath string

	updateEvery        int
	firstSampleTimeout time.Duration

	cmd  *exec.Cmd
	done chan struct{}

	mux        sync.Mutex
	lastSample string
}

func (e *nvidiaSmiLoopExec) queryGPUInfo() ([]byte, error) {
	select {
	case <-e.done:
		return nil, errors.New("process has already exited")
	default:
	}

	e.mux.Lock()
	defer e.mux.Unlock()

	return []byte(e.lastSample), nil
}

func (e *nvidiaSmiLoopExec) run() error {
	secs := 5
	if e.updateEvery < secs {
		secs = e.updateEvery
	}

	ndrunPath := filepath.Join(buildinfo.NetdataBinDir, "nd-run")
	cmd := exec.Command(ndrunPath, e.binPath, "-q", "-x", "-l", strconv.Itoa(secs))

	e.Debugf("executing '%s'", cmd)

	r, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	firstSample := make(chan struct{}, 1)
	done := make(chan struct{})
	e.cmd = cmd
	e.done = done

	go func() {
		defer close(done)

		var buf bytes.Buffer
		var insideLog bool
		var emptyRows int64
		var outsideLogRows int64

		const unexpectedRowsLimit = 500

		sc := bufio.NewScanner(r)

		for sc.Scan() {
			line := sc.Text()

			if !insideLog {
				outsideLogRows++
			} else {
				outsideLogRows = 0
			}

			if line == "" {
				emptyRows++
			} else {
				emptyRows = 0
			}

			if outsideLogRows >= unexpectedRowsLimit || emptyRows >= unexpectedRowsLimit {
				e.Errorf("unexpected output from nvidia-smi loop: outside log rows %d, empty rows %d", outsideLogRows, emptyRows)
				break
			}

			switch {
			case line == "<nvidia_smi_log>":
				insideLog = true
				buf.Reset()

				buf.WriteString(line)
				buf.WriteByte('\n')
			case line == "</nvidia_smi_log>":
				insideLog = false

				buf.WriteString(line)

				e.mux.Lock()
				e.lastSample = buf.String()
				e.mux.Unlock()

				buf.Reset()

				select {
				case firstSample <- struct{}{}:
				default:
				}
			case insideLog:
				buf.WriteString(line)
				buf.WriteByte('\n')
			default:
				continue
			}
		}
	}()

	select {
	case <-e.done:
		_ = e.stop()
		return errors.New("process exited before the first sample was collected")
	case <-time.After(e.firstSampleTimeout):
		_ = e.stop()
		return errors.New("timed out waiting for first sample")
	case <-firstSample:
		return nil
	}
}

func (e *nvidiaSmiLoopExec) stop() error {
	if e.cmd == nil || e.cmd.Process == nil {
		return nil
	}

	_ = e.cmd.Process.Kill()
	_ = e.cmd.Wait()
	e.cmd = nil

	select {
	case <-e.done:
		return nil
	case <-time.After(time.Second * 2):
		return errors.New("timed out waiting for process to exit")
	}
}
