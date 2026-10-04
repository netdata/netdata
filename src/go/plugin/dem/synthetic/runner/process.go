// SPDX-License-Identifier: GPL-3.0-or-later

package runner

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

type treeProcess interface{ Wait() ndexec.TreeResult }

type startFunc func(context.Context, ndexec.ProcessOptions, string, ...string) (treeProcess, error)

// executeProcess starts all local I/O ownership only after successful TreeStart.
// Its parser can request cancellation, but only TreeResult may prove drainage.
func (e *Engine) executeProcess(
	ctx context.Context,
	args []string,
	input []byte,
	stdout func(io.Reader) error,
	stderr func(io.Reader),
) (ndexec.TreeResult, error) {
	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var files []*os.File
	pipe := func() (*os.File, *os.File, error) {
		read, write, err := os.Pipe()
		if err == nil {
			files = append(files, read, write)
		}
		return read, write, err
	}
	defer func() {
		for _, file := range files {
			_ = file.Close()
		}
	}()
	inRead, inWrite, err := pipe()
	if err != nil {
		return ndexec.TreeResult{
			Drained: true,
		}, err
	}
	outRead, outWrite, err := pipe()
	if err != nil {
		return ndexec.TreeResult{
			Drained: true,
		}, err
	}
	errRead, errWrite, err := pipe()
	if err != nil {
		return ndexec.TreeResult{
			Drained: true,
		}, err
	}
	process, err := e.start(
		processCtx,
		ndexec.ProcessOptions{
			Stdin:  inRead,
			Stdout: outWrite,
			Stderr: errWrite,
		},
		e.config.NodePath,
		args...)
	if err != nil {
		return ndexec.TreeResult{
			Drained: true,
		}, err
	}
	_ = inRead.Close()
	_ = outWrite.Close()
	_ = errWrite.Close()
	var inputErr error
	var readers sync.WaitGroup
	readers.Add(3)
	go func() {
		defer readers.Done()
		defer inWrite.Close()
		if _, err := inWrite.Write(input); err != nil {
			inputErr = err
			cancel()
		}
	}()
	go func() {
		defer readers.Done()
		defer outRead.Close()
		if err := stdout(outRead); err != nil {
			cancel()
		}
	}()
	go func() { defer readers.Done(); defer errRead.Close(); stderr(errRead) }()
	result := process.Wait()
	if !result.Drained {
		e.poison(result.Err)
		// Unknown descendants may still hold inherited writers/readers. Closing only
		// our own ends lets local goroutines join; it is never cleanup evidence.
		_ = inWrite.Close()
		_ = outRead.Close()
		_ = errRead.Close()
	}
	readers.Wait()
	result.Err = errors.Join(result.Err, inputErr)
	return result, nil
}
