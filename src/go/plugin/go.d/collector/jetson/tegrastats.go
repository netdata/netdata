// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// tegrastatsInterval is the sampling interval requested from tegrastats, its documented default.
const tegrastatsInterval = time.Second

func lookupTegrastats() (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("jetson requires Linux")
	}
	path, err := exec.LookPath("tegrastats")
	if err != nil {
		return "", errors.New("tegrastats executable not found in PATH")
	}
	return path, nil
}

// tegrastatsProcess is one owned tegrastats process. It sends each recognized
// record on records until reading fails or the process is closed.
type tegrastatsProcess struct {
	process *ndexec.Process
	stdout  *os.File
	cancel  context.CancelFunc

	records  chan observation
	readDone chan struct{}
	readErr  error
	exited   chan struct{}
	exitErr  error
}

// startTegrastats starts tegrastats through the unprivileged command helper.
// Canceling ctx terminates the process tree; the caller MUST close the result.
func startTegrastats(ctx context.Context, path string) (*tegrastatsProcess, error) {
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("open tegrastats output pipe: %w", err)
	}
	ctx, cancel := context.WithCancel(ctx)
	interval := strconv.FormatInt(tegrastatsInterval.Milliseconds(), 10)
	process, err := ndexec.StartUnprivilegedProcess(ctx, ndexec.ProcessOptions{
		Stdout: childStdout,
	}, path, "--interval", interval)
	// Only the child may keep the write end open, so its exit ends our reads.
	_ = childStdout.Close()
	if err != nil {
		cancel()
		_ = stdout.Close()
		return nil, fmt.Errorf("start tegrastats: %w", err)
	}

	p := &tegrastatsProcess{
		process:  process,
		stdout:   stdout,
		cancel:   cancel,
		records:  make(chan observation),
		readDone: make(chan struct{}),
		exited:   make(chan struct{}),
	}
	go p.readRecords(ctx)
	go func() {
		defer close(p.exited)
		p.exitErr = process.Wait()
	}()
	return p, nil
}

// close terminates the owned process tree and joins its reader and reaper, so a
// replacement never overlaps it.
func (p *tegrastatsProcess) close() {
	p.cancel()
	_ = p.stdout.Close()
	_ = p.process.Close()
	<-p.readDone
	<-p.exited
}

// exitError describes the process exit; valid only after exited is closed.
func (p *tegrastatsProcess) exitError() error {
	if p.exitErr != nil {
		return fmt.Errorf("tegrastats exited: %w", p.exitErr)
	}
	return errors.New("tegrastats exited")
}

// readError describes why reading stopped; valid only after readDone is closed.
func (p *tegrastatsProcess) readError() error {
	return fmt.Errorf("read tegrastats output: %w", p.readErr)
}

func (p *tegrastatsProcess) readRecords(ctx context.Context) {
	defer close(p.readDone)
	r := bufio.NewReader(p.stdout)
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			p.readErr = err
			return
		}
		s, ok := parseRecord(line)
		if !ok {
			continue
		}
		select {
		case p.records <- observation{
			sample: s,
			at:     time.Now(),
		}:
		case <-ctx.Done():
			p.readErr = ctx.Err()
			return
		}
	}
}
