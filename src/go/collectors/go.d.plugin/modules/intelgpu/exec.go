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
)

func newIntelGpuTopExec(binPath string, updateEvery int) (*intelGpuTopExec, error) {
	topExec := &intelGpuTopExec{
		binPath:     binPath,
		updateEvery: updateEvery,
	}

	if err := topExec.run(); err != nil {
		return nil, err
	}

	return topExec, nil
}

type intelGpuTopExec struct {
	binPath     string
	updateEvery int

	cmd  *exec.Cmd
	done chan struct{}

	mux        sync.Mutex
	lastSample string
}

func (e *intelGpuTopExec) run() error {
	refresh := 900
	if e.updateEvery > 1 {
		refresh = e.updateEvery*1000 - 500 // milliseconds
	}

	cmd := exec.Command(e.binPath, "-J", "-s", strconv.Itoa(refresh))

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

			if buf.Cap() == 0 && text != "{" || text == "" {
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
	case <-time.After(time.Second * 3):
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
	_, _ = e.cmd.Process.Wait()
	e.cmd = nil

	select {
	case <-e.done:
		return nil
	case <-time.After(time.Second * 2):
		return errors.New("timed out waiting for process to exit")
	}
}
