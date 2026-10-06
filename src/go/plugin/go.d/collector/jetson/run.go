// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type sourceTiming struct {
	freshFor   time.Duration
	stallAfter time.Duration
	retryMin   time.Duration
	retryMax   time.Duration
}

type observation struct {
	sample sample
	at     time.Time
}

func (c *Collector) run(ctx context.Context, ready func()) error {
	ctx, cancel := context.WithCancel(ctx)
	c.runMu.Lock()
	if c.cancel != nil {
		c.runMu.Unlock()
		cancel()
		return errors.New("tegrastats acquisition is already running")
	}
	c.cancel = cancel
	c.runMu.Unlock()
	defer func() {
		cancel()
		c.latest.Store(nil)
		c.runMu.Lock()
		c.cancel = nil
		c.runMu.Unlock()
	}()
	if c.binary == "" {
		return errors.New("tegrastats executable has not been resolved")
	}

	started := false
	retry := c.timing.retryMin
	for ctx.Err() == nil {
		since := time.Now()
		observed, err := c.runSource(ctx, func() {
			if !started {
				started = true
				ready()
			}
		})
		if ctx.Err() != nil {
			return nil
		}
		// Startup failures belong to the managed runtime. After readiness, keep
		// recovery inside Run: returning would permanently stop this job.
		if !started {
			return err
		}
		c.Limit("tegrastats:source", 1, time.Minute).Warningf("tegrastats source unavailable: %v", err)
		if observed && time.Since(since) >= c.timing.stallAfter {
			retry = c.timing.retryMin
		}
		timer := time.NewTimer(retry)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		retry = min(retry*2, c.timing.retryMax)
	}
	return nil
}

func (c *Collector) runSource(ctx context.Context, ready func()) (bool, error) {
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		return false, fmt.Errorf("open tegrastats output pipe: %w", err)
	}
	process, err := ndexec.StartUnprivilegedProcess(ctx, ndexec.ProcessOptions{
		Stdout: writeEnd,
	}, c.binary, "--interval", "1000")
	_ = writeEnd.Close()
	if err != nil {
		_ = readEnd.Close()
		return false, fmt.Errorf("start tegrastats: %w", err)
	}
	readCtx, cancelRead := context.WithCancel(ctx)
	samples := make(chan observation)
	readDone := make(chan struct{})
	waitDone := make(chan struct{})
	var readErr, waitErr error
	go func() { defer close(readDone); readErr = readSamples(readCtx, readEnd, samples) }()
	go func() { defer close(waitDone); waitErr = process.Wait() }()
	defer func() {
		c.latest.Store(nil)
		cancelRead()
		_ = readEnd.Close()
		_ = process.Close()
		// Join the reader and the owned process before starting a replacement.
		<-readDone
		<-waitDone
	}()
	ready()
	watchdog := time.NewTimer(c.timing.stallAfter)
	defer watchdog.Stop()
	observed := false
	for {
		select {
		case <-ctx.Done():
			return observed, nil
		case <-waitDone:
			return observed, fmt.Errorf("tegrastats exited: %v", waitErr)
		case <-readDone:
			return observed, fmt.Errorf("read tegrastats output: %w", readErr)
		case next := <-samples:
			observed = true
			c.latest.Store(&next)
			watchdog.Reset(c.timing.stallAfter)
		case <-watchdog.C:
			return observed, errors.New("tegrastats stopped producing recognizable samples")
		}
	}
}

func readSamples(ctx context.Context, pipe *os.File, samples chan<- observation) error {
	reader := bufio.NewReader(pipe)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		value, ok := parseSample(line)
		if !ok {
			continue
		}
		next := observation{
			sample: value,
			at:     time.Now(),
		}
		select {
		case samples <- next:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
