// SPDX-License-Identifier: GPL-3.0-or-later
package otelcolpoc

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// workerSession owns one process and its parent-lifetime pipe. The worker
// gracefully shuts down on EOF; ndexec contains/reaps it if that fails.
type workerSession struct {
	process            *ndexec.Process
	stdin, stdout      *os.File
	ready              chan error
	readerDone, exited chan struct{}
	waitErr            error // Published by exited.
}

func startWorker(binary string, validate bool) (*workerSession, error) {
	childIn, stdin, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	stdout, childOut, err := os.Pipe()
	if err != nil {
		childIn.Close()
		stdin.Close()
		return nil, err
	}
	args := []string{"--config=worker:stdin"}
	if validate {
		args = append([]string{"validate"}, args...)
	}
	process, err := ndexec.StartUnprivilegedProcess(
		context.Background(),
		ndexec.ProcessOptions{
			Stdin:  childIn,
			Stdout: childOut,
			Stderr: os.Stderr,
		},
		binary,
		args...)
	childIn.Close()
	childOut.Close()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return nil, err
	}
	s := &workerSession{
		process:    process,
		stdin:      stdin,
		stdout:     stdout,
		ready:      make(chan error, 1),
		readerDone: make(chan struct{}),
		exited:     make(chan struct{}),
	}
	go func() { s.waitErr = process.Wait(); close(s.exited) }()
	go func() {
		defer close(s.readerDone)
		scanner := bufio.NewScanner(stdout)
		if !scanner.Scan() {
			s.ready <- fmt.Errorf("Collector readiness stream ended: %v", scanner.Err())
			return
		}
		var pid int
		if n, err := fmt.Sscanf(scanner.Text(), "NETDATA_WORKER_READY %d", &pid); n != 1 || err != nil || pid <= 0 {
			s.ready <- fmt.Errorf("invalid Collector readiness message")
			return
		}
		s.ready <- nil
		// Drain until exit; worker output never reaches the Agent's protocol stream.
		for scanner.Scan() {
		}
	}()
	return s, nil
}
func (s *workerSession) write(ctx context.Context, payload []byte) error {
	done := make(chan error, 1)
	go func() { _, err := s.stdin.Write(append(append([]byte(nil), payload...), '\n')); done <- err }()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		s.stdin.Close()
		<-done
		return ctx.Err()
	}
}
func (s *workerSession) close() {
	s.stdin.Close()
	select {
	case <-s.exited:
	case <-time.After(5 * time.Second):
		_ = s.process.Close()
	}
	s.stdout.Close()
	<-s.readerDone
	<-s.exited
}
