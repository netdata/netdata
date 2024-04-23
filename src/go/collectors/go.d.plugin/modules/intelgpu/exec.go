// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"bufio"
	"bytes"
	"errors"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/logger"
)

func newIntelGpuTopExec(ndsudoPath string, updateEvery int, log *logger.Logger) (*intelGpuTopExec, error) {
	topExec := &intelGpuTopExec{
		Logger:             log,
		ndsudoPath:         ndsudoPath,
		updateEvery:        updateEvery,
		firstSampleTimeout: time.Second * 3,
	}

	if err := topExec.run(); err != nil {
		return nil, err
	}

	return topExec, nil
}

type intelGpuTopExec struct {
	*logger.Logger

	ndsudoPath         string
	updateEvery        int
	firstSampleTimeout time.Duration

	cmd  *exec.Cmd
	done chan struct{}

	mux        sync.Mutex
	lastSample string
}

func (e *intelGpuTopExec) run() error {
	cmd := exec.Command(e.ndsudoPath, "igt-json", "--interval", e.calcIntervalArg())

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
		sc := bufio.NewScanner(r)
		var buf bytes.Buffer
		var n int

		for sc.Scan() {
			if n++; n > 1000 {
				break
			}

			text := sc.Text()

			if buf.Len() == 0 && text != "{" || text == "" {
				continue
			}

			if text == "}," {
				text = "}"
			}

			buf.WriteString(text + "\n")

			if text[0] == '}' {
				e.mux.Lock()
				e.lastSample = buf.String()
				e.mux.Unlock()

				select {
				case firstSample <- struct{}{}:
				default:
				}

				buf.Reset()
				n = 0
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

func (e *intelGpuTopExec) queryGPUSummaryJson() ([]byte, error) {
	select {
	case <-e.done:
		return nil, errors.New("process has already exited")
	default:
	}

	e.mux.Lock()
	defer e.mux.Unlock()

	return []byte(e.lastSample), nil
}

func (e *intelGpuTopExec) stop() error {
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

func (e *intelGpuTopExec) calcIntervalArg() string {
	// intel_gpu_top appends the end marker ("},\n") of the previous sample to the beginning of the next sample.
	// interval must be < than 'firstSampleTimeout'
	var interval int
	m := min(e.updateEvery, int(e.firstSampleTimeout.Seconds()))
	if m <= 1 {
		interval = 900
	} else {
		interval = m*1000 - 500 // milliseconds
	}
	return strconv.Itoa(interval)
}
